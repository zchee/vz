package vz

import (
	"github.com/Code-Hex/vz/v3/internal/objc"
)

// VirtualMachineConfiguration defines the configuration of a VirtualMachine.
//
// The following properties must be configured before creating a virtual machine:
//   - bootLoader
//
// The configuration of devices is often done in two parts:
// - Device configuration
// - Device attachment
//
// The device configuration defines the characteristics of the emulated hardware device.
// For example, for a network device, the device configuration defines the type of network adapter present
// in the virtual machine and its MAC address.
//
// The device attachment defines the host machine's resources that are exposed by the virtual device.
// For example, for a network device, the device attachment can be virtual network interface with a NAT
// to the real network.
//
// Creating a virtual machine using the Virtualization framework requires the app to have the "com.apple.security.virtualization" entitlement.
// A VirtualMachineConfiguration is considered invalid if the application does not have the entitlement.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachineconfiguration?language=objc
type VirtualMachineConfiguration struct {
	cpuCount   uint
	memorySize uint64
	*pointer

	networkDeviceConfiguration []*VirtioNetworkDeviceConfiguration
	storageDeviceConfiguration []StorageDeviceConfiguration
	usbControllerConfiguration []USBControllerConfiguration
}

// NewVirtualMachineConfiguration creates a new configuration.
//
//   - bootLoader parameter is used when the virtual machine starts.
//   - cpu parameter is The number of CPUs must be a value between
//     VZVirtualMachineConfiguration.minimumAllowedCPUCount and VZVirtualMachineConfiguration.maximumAllowedCPUCount.
//   - memorySize parameter represents memory size in bytes.
//     The memory size must be a multiple of a 1 megabyte (1024 * 1024 bytes) between
//     VZVirtualMachineConfiguration.minimumAllowedMemorySize and VZVirtualMachineConfiguration.maximumAllowedMemorySize.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtualMachineConfiguration(bootLoader BootLoader, cpu uint, memorySize uint64) (*VirtualMachineConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	configPtr := objc.New("VZVirtualMachineConfiguration", "init")
	objc.SendVoid(configPtr, "setBootLoader:", objc.Ptr(bootLoader))
	objc.SendVoid(configPtr, "setCPUCount:", cpu)
	objc.SendVoid(configPtr, "setMemorySize:", memorySize)
	config := &VirtualMachineConfiguration{
		cpuCount:   cpu,
		memorySize: memorySize,
		pointer:    objc.NewPointer(configPtr),
	}
	objc.SetFinalizer(config, func(self *VirtualMachineConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

// Validate the configuration.
//
// Return true if the configuration is valid.
// If error is not nil, assigned with the validation error if the validation failed.
func (v *VirtualMachineConfiguration) Validate() (bool, error) {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	ret := objc.Send[bool](
		objc.ID(uintptr(objc.Ptr(v))),
		objc.RegisterName("validateWithError:"),
		errSlot,
	)
	if objc.HasError(errSlot) {
		return false, newNSError(objc.ErrorFromSlot(errSlot))
	}
	return ret, nil
}

// SetEntropyDevicesVirtualMachineConfiguration sets list of entropy devices. Empty by default.
func (v *VirtualMachineConfiguration) SetEntropyDevicesVirtualMachineConfiguration(cs []*VirtioEntropyDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setEntropyDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetMemoryBalloonDevicesVirtualMachineConfiguration sets list of memory balloon devices. Empty by default.
func (v *VirtualMachineConfiguration) SetMemoryBalloonDevicesVirtualMachineConfiguration(cs []MemoryBalloonDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setMemoryBalloonDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetNetworkDevicesVirtualMachineConfiguration sets list of network adapters. Empty by default.
func (v *VirtualMachineConfiguration) SetNetworkDevicesVirtualMachineConfiguration(cs []*VirtioNetworkDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setNetworkDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
	v.networkDeviceConfiguration = cs
}

// NetworkDevices return the list of network device configuration set in this virtual machine configuration.
// Return an empty array if no network device configuration is set.
func (v *VirtualMachineConfiguration) NetworkDevices() []*VirtioNetworkDeviceConfiguration {
	return v.networkDeviceConfiguration
}

// SetSerialPortsVirtualMachineConfiguration sets list of serial ports. Empty by default.
func (v *VirtualMachineConfiguration) SetSerialPortsVirtualMachineConfiguration(cs []*VirtioConsoleDeviceSerialPortConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setSerialPorts:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetSocketDevicesVirtualMachineConfiguration sets list of socket devices. Empty by default.
func (v *VirtualMachineConfiguration) SetSocketDevicesVirtualMachineConfiguration(cs []SocketDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setSocketDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SocketDevices return the list of socket device configuration configured in this virtual machine configuration.
// Return an empty array if no socket device configuration is set.
func (v *VirtualMachineConfiguration) SocketDevices() []SocketDeviceConfiguration {
	nsArray := objc.NewNSArray(
		objc.SendPtr(objc.Ptr(v), "socketDevices"),
	)
	ptrs := nsArray.ToPointerSlice()
	socketDevices := make([]SocketDeviceConfiguration, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = newVirtioSocketDeviceConfiguration(ptr)
	}
	return socketDevices
}

// SetStorageDevicesVirtualMachineConfiguration sets list of disk devices. Empty by default.
func (v *VirtualMachineConfiguration) SetStorageDevicesVirtualMachineConfiguration(cs []StorageDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setStorageDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
	v.storageDeviceConfiguration = cs
}

// StorageDevices return the list of storage device configuration configured in this virtual machine configuration.
// Return an empty array if no storage device configuration is set.
func (v *VirtualMachineConfiguration) StorageDevices() []StorageDeviceConfiguration {
	return v.storageDeviceConfiguration
}

// SetCustomVirtioDevicesVirtualMachineConfiguration sets the list of custom Virtio
// devices. Empty by default.
//
// Custom Virtio devices require macOS 27. Because the elements can only be created
// with NewCustomVirtioDeviceConfiguration (which enforces the version), this setter
// is intentionally not version-gated.
func (v *VirtualMachineConfiguration) SetCustomVirtioDevicesVirtualMachineConfiguration(cs []*CustomVirtioDeviceConfiguration) {
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	// Unlike SetStorageDevices/SetUSBDevices, the Go slice is not retained on the
	// receiver: there is no CustomVirtioDevices() getter, and the NSArray copy the
	// configuration holds already retains the element objects.
	objc.SendVoid(objc.Ptr(v), "setCustomVirtioDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetDirectorySharingDevicesVirtualMachineConfiguration sets list of directory sharing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetDirectorySharingDevicesVirtualMachineConfiguration(cs []DirectorySharingDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setDirectorySharingDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetPlatformVirtualMachineConfiguration sets the hardware platform to use. Defaults to GenericPlatformConfiguration.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPlatformVirtualMachineConfiguration(c PlatformConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	objc.SendVoid(objc.Ptr(v), "setPlatform:", objc.Ptr(c))
}

// SetGraphicsDevicesVirtualMachineConfiguration sets list of graphics devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetGraphicsDevicesVirtualMachineConfiguration(cs []GraphicsDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setGraphicsDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetPointingDevicesVirtualMachineConfiguration sets list of pointing devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetPointingDevicesVirtualMachineConfiguration(cs []PointingDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setPointingDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetKeyboardsVirtualMachineConfiguration sets list of keyboards. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetKeyboardsVirtualMachineConfiguration(cs []KeyboardConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setKeyboards:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetAudioDevicesVirtualMachineConfiguration sets list of audio devices. Empty by default.
//
// This is only supported on macOS 12 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetAudioDevicesVirtualMachineConfiguration(cs []AudioDeviceConfiguration) {
	if err := macOSAvailable(12); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setAudioDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetConsoleDevicesVirtualMachineConfiguration sets list of console devices. Empty by default.
//
// This is only supported on macOS 13 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetConsoleDevicesVirtualMachineConfiguration(cs []ConsoleDeviceConfiguration) {
	if err := macOSAvailable(13); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(cs))
	for i, val := range cs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setConsoleDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// SetUSBControllerConfiguration sets list of USB controllers. Empty by default.
//
// This is only supported on macOS 15 and newer. Older versions do nothing.
func (v *VirtualMachineConfiguration) SetUSBControllersVirtualMachineConfiguration(us []USBControllerConfiguration) {
	if err := macOSAvailable(15); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(us))
	for i, val := range us {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setUsbControllers:", objc.SendPtr(objc.Ptr(array), "copy"))
	v.usbControllerConfiguration = us
}

// USBControllers return the list of usb controller configuration configured in this virtual machine configuration.
// Return an empty array if no usb controller configuration is set.
func (v *VirtualMachineConfiguration) USBControllers() []USBControllerConfiguration {
	return v.usbControllerConfiguration
}

// VirtualMachineConfigurationMinimumAllowedMemorySize returns minimum
// amount of memory required by virtual machines.
func VirtualMachineConfigurationMinimumAllowedMemorySize() uint64 {
	return objc.Send[uint64](
		objc.ID(objc.GetClass("VZVirtualMachineConfiguration")),
		objc.RegisterName("minimumAllowedMemorySize"),
	)
}

// VirtualMachineConfigurationMaximumAllowedMemorySize returns maximum
// amount of memory allowed for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedMemorySize() uint64 {
	return objc.Send[uint64](
		objc.ID(objc.GetClass("VZVirtualMachineConfiguration")),
		objc.RegisterName("maximumAllowedMemorySize"),
	)
}

// VirtualMachineConfigurationMinimumAllowedCPUCount returns minimum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMinimumAllowedCPUCount() uint {
	return uint(objc.Send[uint64](
		objc.ID(objc.GetClass("VZVirtualMachineConfiguration")),
		objc.RegisterName("minimumAllowedCPUCount"),
	))
}

// VirtualMachineConfigurationMaximumAllowedCPUCount returns maximum
// number of CPUs for a virtual machine.
func VirtualMachineConfigurationMaximumAllowedCPUCount() uint {
	return uint(objc.Send[uint64](
		objc.ID(objc.GetClass("VZVirtualMachineConfiguration")),
		objc.RegisterName("maximumAllowedCPUCount"),
	))
}
