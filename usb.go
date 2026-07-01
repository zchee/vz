package vz

import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// NewUSBMassStorageDevice initialize the runtime USB Mass Storage device object.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewUSBMassStorageDevice(config *USBMassStorageDeviceConfiguration) (USBDevice, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}
	ptr := objc.New("VZUSBMassStorageDevice", "initWithConfiguration:", objc.Ptr(config))
	return newUSBDevice(ptr), nil
}

// USBControllerConfiguration for a usb controller configuration.
type USBControllerConfiguration interface {
	objc.NSObject

	usbControllerConfiguration()
}

type baseUSBControllerConfiguration struct{}

func (*baseUSBControllerConfiguration) usbControllerConfiguration() {}

// USBDeviceConfiguration is an interface for a USB device configuration that a
// USB controller can start with (VZUSBDeviceConfiguration).
//
// see: https://developer.apple.com/documentation/virtualization/vzusbdeviceconfiguration?language=objc
type USBDeviceConfiguration interface {
	objc.NSObject

	usbDeviceConfiguration()
}

type baseUSBDeviceConfiguration struct{}

func (*baseUSBDeviceConfiguration) usbDeviceConfiguration() {}

// XHCIControllerConfiguration is a configuration of the USB XHCI controller.
//
// This configuration creates a USB XHCI controller device for the guest.
// see: https://developer.apple.com/documentation/virtualization/vzxhcicontrollerconfiguration?language=objc
type XHCIControllerConfiguration struct {
	*pointer

	*baseUSBControllerConfiguration

	usbDevices []USBDeviceConfiguration
}

var _ USBControllerConfiguration = (*XHCIControllerConfiguration)(nil)

// NewXHCIControllerConfiguration creates a new XHCIControllerConfiguration.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
func NewXHCIControllerConfiguration() (*XHCIControllerConfiguration, error) {
	if err := macOSAvailable(15); err != nil {
		return nil, err
	}

	config := &XHCIControllerConfiguration{
		pointer: objc.NewPointer(objc.New("VZXHCIControllerConfiguration", "init")),
	}

	objc.SetFinalizer(config, func(self *XHCIControllerConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

// SetUSBDevices sets the list of USB devices the controller starts with. Each
// device is created as a runtime object in the controller's usbDevices property
// when the virtual machine starts.
//
// This is only supported on macOS 15 and newer; older versions do nothing.
//
// see: https://developer.apple.com/documentation/virtualization/vzusbcontrollerconfiguration/usbdevices?language=objc
func (c *XHCIControllerConfiguration) SetUSBDevices(devices []USBDeviceConfiguration) {
	if err := macOSAvailable(15); err != nil {
		return
	}
	ptrs := make([]objc.NSObject, len(devices))
	for i, d := range devices {
		ptrs[i] = d
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(c), "setUsbDevices:", objc.SendPtr(objc.Ptr(array), "copy"))
	// Retain the Go configs for the controller config's lifetime so their
	// Objective-C objects are not finalized while VZ holds the copied array.
	c.usbDevices = devices
}

// USBController is representing a USB controller in a virtual machine.
type USBController struct {
	dispatchQueue unsafe.Pointer
	*pointer
}

func newUSBController(ptr, dispatchQueue unsafe.Pointer) *USBController {
	return &USBController{
		dispatchQueue: dispatchQueue,
		pointer:       objc.NewPointer(ptr),
	}
}

// attachDetach drives a USB attach or detach through the controller's queue and
// waits for the completion handler.
func (u *USBController) attachDetach(selector string, device USBDevice) error {
	return completionCall(u.dispatchQueue, objc.Ptr(u), selector, objc.Ptr(device))
}

// Attach attaches a USB device.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
//
// If the device is successfully attached to the controller, it will appear in the usbDevices property,
// its usbController property will be set to point to the USB controller that it is attached to
// and completion handler will return nil.
// If the device was previously attached to this or another USB controller, attach function will fail
// with the `vz.ErrorDeviceAlreadyAttached`. If the device cannot be initialized correctly, attach
// function will fail with `vz.ErrorDeviceInitializationFailure`.
func (u *USBController) Attach(device USBDevice) error {
	if err := macOSAvailable(15); err != nil {
		return err
	}
	return u.attachDetach("attachDevice:completionHandler:", device)
}

// Detach detaches a USB device.
//
// This is only supported on macOS 15 and newer, error will
// be returned on older versions.
//
// If the device is successfully detached from the controller, it will disappear from the usbDevices property,
// its usbController property will be set to nil and completion handler will return nil.
// If the device wasn't attached to the controller at the time of calling detach method, it will fail
// with the `vz.ErrorDeviceNotFound` error.
func (u *USBController) Detach(device USBDevice) error {
	if err := macOSAvailable(15); err != nil {
		return err
	}
	return u.attachDetach("detachDevice:completionHandler:", device)
}

// USBDevices return a list of USB devices attached to controller.
//
// This is only supported on macOS 15 and newer, nil will
// be returned on older versions.
func (u *USBController) USBDevices() []USBDevice {
	if err := macOSAvailable(15); err != nil {
		return nil
	}
	nsArray := objc.NewNSArray(
		objc.SendPtr(objc.Ptr(u), "usbDevices"),
	)
	ptrs := nsArray.ToPointerSlice()
	usbDevices := make([]USBDevice, len(ptrs))
	for i, ptr := range ptrs {
		usbDevices[i] = newUSBDevice(ptr)
	}
	return usbDevices
}

// USBDevice is an interface that represents a USB device in a VM.
type USBDevice interface {
	objc.NSObject

	UUID() string

	usbDevice()
}

func newUSBDevice(ptr unsafe.Pointer) *usbDevice {
	return &usbDevice{
		pointer: objc.NewPointer(ptr),
	}
}

type usbDevice struct {
	*pointer
}

func (*usbDevice) usbDevice() {}

var _ USBDevice = (*usbDevice)(nil)

// UUID returns the device UUID.
func (u *usbDevice) UUID() string {
	uuid := objc.SendPtr(objc.Ptr(u), "uuid") // NSUUID
	str := objc.SendPtr(uuid, "UUIDString")   // NSString
	return objc.GoString(objc.SendPtr(str, "UTF8String"))
}
