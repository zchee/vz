package vz

import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// VirtioSharedMemoryRegionConfiguration defines a Virtio shared-memory region: a window
// of host memory continuously shared between a custom Virtio device implementation and
// the guest, identified by a device-specific region ID. Attach regions to a
// CustomVirtioDeviceConfiguration with SetSharedMemoryRegions, then map and unmap host
// memory into a region at runtime through VirtioSharedMemoryRegion.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregionconfiguration?language=objc
type VirtioSharedMemoryRegionConfiguration struct {
	*pointer
}

// NewVirtioSharedMemoryRegionConfiguration creates a shared-memory region configuration
// with the given device-specific region ID and size in bytes.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func NewVirtioSharedMemoryRegionConfiguration(regionID uint8, size uint64) (*VirtioSharedMemoryRegionConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	config := &VirtioSharedMemoryRegionConfiguration{
		pointer: objc.NewPointer(
			objc.New("VZVirtioSharedMemoryRegionConfiguration", "initWithRegionID:size:", regionID, size),
		),
	}
	objc.SetFinalizer(config, func(self *VirtioSharedMemoryRegionConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

// RegionID returns the device-specific shared-memory region ID.
func (c *VirtioSharedMemoryRegionConfiguration) RegionID() uint8 {
	return objc.Send[uint8](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("regionID"))
}

// Size returns the size of the shared-memory region in bytes.
func (c *VirtioSharedMemoryRegionConfiguration) Size() uint64 {
	return objc.Send[uint64](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("size"))
}

// SetSharedMemoryRegions sets the list of shared-memory regions the device advertises to
// the guest. Empty by default; at most MaximumAllowedSharedMemoryRegionCount regions may
// be declared (enforced at VirtualMachineConfiguration.Validate).
//
// Like SetCustomVirtioDevicesVirtualMachineConfiguration, the Go slice is not retained on
// the receiver: there is no getter, and the NSArray copy the configuration holds already
// retains the element objects.
func (c *CustomVirtioDeviceConfiguration) SetSharedMemoryRegions(regions []*VirtioSharedMemoryRegionConfiguration) {
	ptrs := make([]objc.NSObject, len(regions))
	for i, r := range regions {
		ptrs[i] = r
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(c), "setSharedMemoryRegions:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// MaximumAllowedSharedMemoryRegionCount returns the maximum number of shared-memory
// regions a custom Virtio device configuration may declare.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func MaximumAllowedSharedMemoryRegionCount() (uint, error) {
	if err := macOSAvailable(27); err != nil {
		return 0, err
	}
	return uint(objc.Send[uint64](
		objc.ID(objc.GetClass("VZCustomVirtioDeviceConfiguration")),
		objc.RegisterName("maximumAllowedSharedMemoryRegionCount"),
	)), nil
}

// VirtioSharedMemoryRegion is a runtime Virtio shared-memory region belonging to a custom
// Virtio device, obtained from CustomVirtioDevice.SharedMemoryRegions. Use MapMemory and
// UnmapMemory to map host memory into and out of the region's guest-visible window during
// the virtual machine's run.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion?language=objc
type VirtioSharedMemoryRegion struct {
	*pointer
	deviceQueue unsafe.Pointer
}

// newVirtioSharedMemoryRegion wraps a framework-owned VZVirtioSharedMemoryRegion (a +0
// element of the device's sharedMemoryRegions array), retaining it so the wrapper can
// outlive a single access and releasing it in the finalizer. deviceQueue is the device's
// serial queue, on which map/unmap are dispatched.
func newVirtioSharedMemoryRegion(ptr, deviceQueue unsafe.Pointer) *VirtioSharedMemoryRegion {
	objc.SendVoid(ptr, "retain")
	r := &VirtioSharedMemoryRegion{pointer: objc.NewPointer(ptr), deviceQueue: deviceQueue}
	objc.SetFinalizer(r, func(self *VirtioSharedMemoryRegion) {
		objc.Release(self)
	})
	return r
}

// RegionID returns the device-specific shared-memory region ID.
func (r *VirtioSharedMemoryRegion) RegionID() uint8 {
	return objc.Send[uint8](objc.ID(uintptr(objc.Ptr(r))), objc.RegisterName("regionID"))
}

// Size returns the size of the shared-memory region in bytes.
func (r *VirtioSharedMemoryRegion) Size() uint64 {
	return objc.Send[uint64](objc.ID(uintptr(objc.Ptr(r))), objc.RegisterName("size"))
}

// MapMemory maps a chunk of host memory into the shared-memory region at the given
// offset, and blocks until the framework reports completion or an error.
//
// memory, offset, and size must all be aligned to the host page size. memory is
// caller-owned host memory (not a Go slice) that must stay valid and mapped for as long
// as the region uses it — the binding does not take ownership of it.
//
// MapMemory dispatches onto and waits on the device queue, so it must NOT be called from
// within a CustomVirtioHandler callback (which already runs on that serial queue) —
// doing so deadlocks. Call it from another goroutine.
func (r *VirtioSharedMemoryRegion) MapMemory(memory unsafe.Pointer, offset, size uint64) error {
	return completionCall(r.deviceQueue, objc.Ptr(r),
		"mapMemory:atOffset:size:completionHandler:", memory, offset, size)
}

// UnmapMemory unmaps a chunk of host memory from the shared-memory region at the given
// offset, and blocks until completion or an error. offset and size must be host-page
// aligned. Like MapMemory, it must not be called from within a device-queue callback.
func (r *VirtioSharedMemoryRegion) UnmapMemory(offset, size uint64) error {
	return completionCall(r.deviceQueue, objc.Ptr(r),
		"unmapMemoryAtOffset:size:completionHandler:", offset, size)
}

// SharedMemoryRegions returns the runtime shared-memory regions of the device, in the
// order they were configured. It is valid once the device has been created; unlike
// QueueAtIndex/NegotiatedFeatures it does not require the guest driver to have reached
// DRIVER_OK.
func (d *CustomVirtioDevice) SharedMemoryRegions() []*VirtioSharedMemoryRegion {
	arr := objc.NewNSArray(objc.SendPtr(objc.Ptr(d), "sharedMemoryRegions"))
	ptrs := arr.ToPointerSlice()
	out := make([]*VirtioSharedMemoryRegion, len(ptrs))
	for i, p := range ptrs {
		out[i] = newVirtioSharedMemoryRegion(p, d.queue)
	}
	return out
}
