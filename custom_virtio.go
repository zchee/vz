package vz

import (
	"sync/atomic"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// CustomVirtioDeviceConfiguration defines the configuration of a custom Virtio
// device that the Virtualization framework creates when the virtual machine
// starts. Configure the four discovery properties a guest needs to detect the
// device — the device ID, the PCI class and subclass IDs, and the virtqueue count —
// together with the optional feature sets and device-specific configuration.
//
// This wraps the configuration-time surface only. The device provider, the runtime
// VZCustomVirtioDevice, the virtqueues and guest-memory access are provided
// separately.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodeviceconfiguration?language=objc
type CustomVirtioDeviceConfiguration struct {
	*pointer

	// delegate and queue are the +1 Go-backed delegate object and the +1 device dispatch
	// queue installed by SetHandler. The delegate provider holds only a WEAK reference to
	// the delegate, so this +1 is its sole keep-alive.
	//
	// IMPORTANT (B2 scaffold): they are anchored to this configuration only because this
	// slice never starts a virtual machine. Plan step 8 requires the keep-alive to be
	// anchored to the VM-run-lifetime *CustomVirtioDevice wrapper, NOT the config — a
	// config-anchored release frees the delegate mid-run and silently kills callbacks
	// (pre-mortem #2). Before any VM starts, B3 MUST build that device wrapper in
	// customVirtioConfiguration:didCreateDevice:, re-anchor these two +1 references to it,
	// and set transferred=true so this finalizer stops owning them.
	delegate    unsafe.Pointer
	queue       unsafe.Pointer
	transferred atomic.Bool // set by the didCreateDevice: transfer; read by the finalizer on a GC goroutine, hence atomic
}

// NewCustomVirtioDeviceConfiguration creates a new custom Virtio device
// configuration.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func NewCustomVirtioDeviceConfiguration() (*CustomVirtioDeviceConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	config := &CustomVirtioDeviceConfiguration{
		pointer: objc.NewPointer(
			objc.New("VZCustomVirtioDeviceConfiguration", "init"),
		),
	}
	objc.SetFinalizer(config, func(self *CustomVirtioDeviceConfiguration) {
		if !self.transferred.Load() {
			if self.delegate != nil {
				objc.Disassociate(uintptr(self.delegate))
				objc.SendVoid(self.delegate, "release")
			}
			if self.queue != nil {
				objc.ReleaseDispatch(self.queue)
			}
		}
		objc.Release(self)
	})
	return config, nil
}

// SetDeviceID sets the Virtio device ID of the device.
func (c *CustomVirtioDeviceConfiguration) SetDeviceID(deviceID uint16) {
	objc.SendVoid(objc.Ptr(c), "setDeviceID:", deviceID)
}

// DeviceID returns the Virtio device ID of the device.
func (c *CustomVirtioDeviceConfiguration) DeviceID() uint16 {
	return objc.Send[uint16](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("deviceID"))
}

// SetPCIClassID sets the PCI class ID of the device.
func (c *CustomVirtioDeviceConfiguration) SetPCIClassID(classID uint8) {
	objc.SendVoid(objc.Ptr(c), "setPCIClassID:", classID)
}

// PCIClassID returns the PCI class ID of the device.
func (c *CustomVirtioDeviceConfiguration) PCIClassID() uint8 {
	return objc.Send[uint8](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("PCIClassID"))
}

// SetPCISubclassID sets the PCI subclass ID of the device.
func (c *CustomVirtioDeviceConfiguration) SetPCISubclassID(subclassID uint8) {
	objc.SendVoid(objc.Ptr(c), "setPCISubclassID:", subclassID)
}

// PCISubclassID returns the PCI subclass ID of the device.
func (c *CustomVirtioDeviceConfiguration) PCISubclassID() uint8 {
	return objc.Send[uint8](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("PCISubclassID"))
}

// SetVirtioQueueCount sets the number of virtqueues (Virtio queues) on the device.
func (c *CustomVirtioDeviceConfiguration) SetVirtioQueueCount(count uint16) {
	objc.SendVoid(objc.Ptr(c), "setVirtioQueueCount:", count)
}

// VirtioQueueCount returns the number of virtqueues (Virtio queues) on the device.
func (c *CustomVirtioDeviceConfiguration) VirtioQueueCount() uint16 {
	return objc.Send[uint16](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("virtioQueueCount"))
}

// MandatoryFeatures returns the set of mandatory features that the device offers and
// the guest must accept. Mutate the returned VirtioFeatureSet to configure the
// mandatory feature bits; the returned object is the configuration's own feature
// set, so mutations are reflected in the configuration.
func (c *CustomVirtioDeviceConfiguration) MandatoryFeatures() *VirtioFeatureSet {
	return newVirtioFeatureSet(objc.SendPtr(objc.Ptr(c), "mandatoryFeatures"))
}

// OptionalFeatures returns the set of optional features that the device offers.
// Mutate the returned VirtioFeatureSet to configure the optional feature bits; the
// returned object is the configuration's own feature set, so mutations are reflected
// in the configuration.
func (c *CustomVirtioDeviceConfiguration) OptionalFeatures() *VirtioFeatureSet {
	return newVirtioFeatureSet(objc.SendPtr(objc.Ptr(c), "optionalFeatures"))
}

// SetDeviceSpecificConfiguration sets the device-specific configuration for the
// device.
func (c *CustomVirtioDeviceConfiguration) SetDeviceSpecificConfiguration(config *VirtioDeviceSpecificConfiguration) {
	objc.SendVoid(objc.Ptr(c), "setDeviceSpecificConfiguration:", objc.Ptr(config))
}

// SetSupportsSaveRestore sets whether the device supports save and restore. It
// defaults to false.
//
// When you enable it, the handler you install with SetHandler must set both
// CustomVirtioHandler.SaveStateForRestore and CustomVirtioHandler.ShouldRestore:
// SaveStateForRestore must return a non-nil slice — return an empty []byte for a
// device with no state to save, never nil, because the framework treats a nil save
// state as a failed save. The Go-backed delegate always implements both Objective-C
// selectors, so the framework's "delegate must respond to the save/restore methods"
// requirement is met regardless; it is the Go callbacks that you must supply.
func (c *CustomVirtioDeviceConfiguration) SetSupportsSaveRestore(supports bool) {
	objc.SendVoid(objc.Ptr(c), "setSupportsSaveRestore:", supports)
}

// SupportsSaveRestore reports whether the device supports save and restore.
func (c *CustomVirtioDeviceConfiguration) SupportsSaveRestore() bool {
	return objc.Send[bool](objc.ID(uintptr(objc.Ptr(c))), objc.RegisterName("supportsSaveRestore"))
}

// VirtioFeatureSet represents a 64-bit set of Virtio feature bits, encoded as two
// 32-bit subsets (subset0 is bits 0–31, subset1 is bits 32–63).
//
// A VirtioFeatureSet is not created directly; obtain one from a
// CustomVirtioDeviceConfiguration via MandatoryFeatures or OptionalFeatures and
// mutate its subsets.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofeatureset?language=objc
type VirtioFeatureSet struct {
	*pointer
}

// newVirtioFeatureSet wraps a VZVirtioFeatureSet the framework owns (a readonly
// property of the configuration). It retains the object so the wrapper can outlive
// the configuration, and releases it in the finalizer.
func newVirtioFeatureSet(ptr unsafe.Pointer) *VirtioFeatureSet {
	objc.SendVoid(ptr, "retain")
	fs := &VirtioFeatureSet{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(fs, func(self *VirtioFeatureSet) {
		objc.Release(self)
	})
	return fs
}

// SetSubset0 sets the feature bits 0 through 31.
func (f *VirtioFeatureSet) SetSubset0(subset uint32) {
	objc.SendVoid(objc.Ptr(f), "setSubset0:", subset)
}

// Subset0 returns the feature bits 0 through 31.
func (f *VirtioFeatureSet) Subset0() uint32 {
	return objc.Send[uint32](objc.ID(uintptr(objc.Ptr(f))), objc.RegisterName("subset0"))
}

// SetSubset1 sets the feature bits 32 through 63.
func (f *VirtioFeatureSet) SetSubset1(subset uint32) {
	objc.SendVoid(objc.Ptr(f), "setSubset1:", subset)
}

// Subset1 returns the feature bits 32 through 63.
func (f *VirtioFeatureSet) Subset1() uint32 {
	return objc.Send[uint32](objc.ID(uintptr(objc.Ptr(f))), objc.RegisterName("subset1"))
}

// VirtioDeviceSpecificConfiguration holds a Virtio device's device-specific
// configuration, serialized into a byte buffer whose layout is defined by the Virtio
// specification for the kind of device you are implementing. Set it on a
// CustomVirtioDeviceConfiguration via SetDeviceSpecificConfiguration.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiodevicespecificconfiguration?language=objc
type VirtioDeviceSpecificConfiguration struct {
	*pointer
}

// NewVirtioDeviceSpecificConfiguration creates a device-specific configuration from
// the serialized configuration data you provide.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func NewVirtioDeviceSpecificConfiguration(configurationData []byte) (*VirtioDeviceSpecificConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	// NSData returns a +1 object; initWithConfigurationData: keeps its own copy (the
	// property is declared copy), so release the +1 NSData once init returns.
	data := objc.NSData(configurationData)
	config := &VirtioDeviceSpecificConfiguration{
		pointer: objc.NewPointer(
			objc.New("VZVirtioDeviceSpecificConfiguration", "initWithConfigurationData:", data),
		),
	}
	objc.SendVoid(data, "release")
	objc.SetFinalizer(config, func(self *VirtioDeviceSpecificConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

// ConfigurationData returns the serialized device-specific configuration bytes.
func (c *VirtioDeviceSpecificConfiguration) ConfigurationData() []byte {
	return objc.NSDataToBytes(objc.SendPtr(objc.Ptr(c), "configurationData"))
}
