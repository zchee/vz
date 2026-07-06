package vz

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// This file wraps the parts of Apple's AccessoryAccess framework needed to
// obtain an AAUSBAccessory, which VZUSBPassthroughDeviceConfiguration captures
// for USB passthrough into a virtual machine (see usb.go).
//
// Resolved from the macOS 27.0 SDK headers before implementation:
//
//   - Entitlement: an app using AAUSBAccessoryManager must be signed with the
//     "com.apple.developer.accessory-access.usb" entitlement, and must be an
//     ordinary (Dock) application, because the manager presents consent UI on
//     the application's behalf. FindUSBAccessories therefore cannot run in a
//     headless/entitlement-less context.
//   - Matching dictionary: AAUSBAccessoryMatchingCriteria requires a real IOKit
//     USB matching dictionary produced by
//     +[IOUSBHostDevice createMatchingDictionaryWithVendorID:productID:...] — a
//     hand-built dictionary of USB property keys is rejected (initWith… returns
//     nil). So the wrapper builds it from vendor/product IDs and loads
//     IOUSBHost.framework in addition to AccessoryAccess.
//   - Capture: the app does NOT open the AAUSBAccessory. The Virtualization
//     framework captures the device itself when the VM starts with the
//     passthrough configuration, or when -[VZUSBController attachDevice:] runs;
//     so this wrapper deliberately omits open/close/XPC.

const (
	accessoryAccessFrameworkPath = "/System/Library/Frameworks/AccessoryAccess.framework/AccessoryAccess"
	ioUSBHostFrameworkPath       = "/System/Library/Frameworks/IOUSBHost.framework/IOUSBHost"
)

var (
	accessoryAccessOnce      sync.Once
	accessoryAccessErr       error
	accessoryListenerGoClass objc.Class
)

// loadAccessoryAccess loads AccessoryAccess.framework and registers the listener
// class on first use. It is safe to call repeatedly.
func loadAccessoryAccess() error {
	accessoryAccessOnce.Do(func() {
		if err := objc.LoadFramework(accessoryAccessFrameworkPath); err != nil {
			accessoryAccessErr = err
			return
		}
		if err := objc.LoadFramework(ioUSBHostFrameworkPath); err != nil {
			accessoryAccessErr = err
			return
		}
		accessoryListenerGoClass, accessoryAccessErr = objc.DefineClass(
			"AAUSBAccessoryListenerGo",
			objc.NSObjectClass(),
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("usbAccessoryDidConnect:"), Fn: accessoryDidConnect},
				{Cmd: objc.RegisterName("usbAccessoryDidDisconnect:"), Fn: accessoryDidDisconnect},
			},
		)
	})
	return accessoryAccessErr
}

// The AAUSBAccessoryListener protocol methods are optional; the initial matches
// arrive through the registration completion handler, so these are no-ops.
func accessoryDidConnect(_ objc.ID, _ objc.SEL, _ unsafe.Pointer)    {}
func accessoryDidDisconnect(_ objc.ID, _ objc.SEL, _ unsafe.Pointer) {}

// USBAccessory represents a USB accessory connected to the host, obtained from
// FindUSBAccessories. Pass it to NewUSBPassthroughDeviceConfiguration to make it
// available to a virtual machine.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessory?language=objc
type USBAccessory struct {
	*pointer
}

// newUSBAccessory wraps and retains an AAUSBAccessory object delivered by the
// framework (delivered objects are autoreleased, so it must be retained to
// outlive the completion handler).
func newUSBAccessory(ptr unsafe.Pointer) *USBAccessory {
	objc.SendVoid(ptr, "retain")
	a := &USBAccessory{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(a, func(self *USBAccessory) {
		objc.Release(self)
	})
	return a
}

// RegistryID returns the IORegistry ID for the USB accessory.
func (a *USBAccessory) RegistryID() uint64 {
	return objc.Send[uint64](objc.ID(uintptr(objc.Ptr(a))), objc.RegisterName("registryID"))
}

// DeviceDescriptor returns the raw USB device descriptor bytes.
func (a *USBAccessory) DeviceDescriptor() []byte {
	return objc.NSDataToBytes(objc.SendPtr(objc.Ptr(a), "deviceDescriptorData"))
}

// ConfigurationDescriptor returns the raw USB configuration descriptor bytes, or
// nil if unavailable.
func (a *USBAccessory) ConfigurationDescriptor() []byte {
	return objc.NSDataToBytes(objc.SendPtr(objc.Ptr(a), "configurationDescriptorData"))
}

// USBAccessoryMatchingCriteria filters USB accessories by their device
// properties for FindUSBAccessories.
//
// see: https://developer.apple.com/documentation/accessoryaccess/aausbaccessorymatchingcriteria?language=objc
type USBAccessoryMatchingCriteria struct {
	*pointer
}

// NewUSBAccessoryMatchingCriteria creates criteria matching a USB accessory by
// its vendor and product IDs. Pass a negative value for either ID to match any.
//
// The matching dictionary is built with
// +[IOUSBHostDevice createMatchingDictionaryWithVendorID:productID:...], which
// AAUSBAccessoryMatchingCriteria requires; a hand-built dictionary of USB
// property keys is rejected.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func NewUSBAccessoryMatchingCriteria(vendorID, productID int) (*USBAccessoryMatchingCriteria, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if err := loadAccessoryAccess(); err != nil {
		return nil, err
	}
	dict := usbMatchingDictionary(vendorID, productID)
	if dict == nil {
		return nil, fmt.Errorf("vz: failed to build USB device matching dictionary")
	}
	ptr := objc.New("AAUSBAccessoryMatchingCriteria", "initWithDeviceMatchingDictionary:", dict)
	objc.SendVoid(dict, "release") // CFRelease the +1 dictionary
	if ptr == nil {
		// The framework rejects criteria creation unless the app is signed with
		// the com.apple.developer.accessory-access.usb entitlement and runs as an
		// ordinary (Dock) application.
		return nil, fmt.Errorf("vz: could not create USB accessory matching criteria (requires the com.apple.developer.accessory-access.usb entitlement and an ordinary application)")
	}
	criteria := &USBAccessoryMatchingCriteria{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(criteria, func(self *USBAccessoryMatchingCriteria) {
		objc.Release(self)
	})
	return criteria, nil
}

// usbMatchingDictionary builds the IOKit USB device matching dictionary that
// AAUSBAccessoryMatchingCriteria requires, using
// +[IOUSBHostDevice createMatchingDictionaryWithVendorID:productID:...]. A nil
// NSNumber argument (negative ID) means "match any". The result is a
// CFMutableDictionaryRef at +1 (toll-free bridged to NSDictionary); the caller
// releases it. Returns nil on failure. This step needs no entitlement.
func usbMatchingDictionary(vendorID, productID int) unsafe.Pointer {
	return objc.Send[unsafe.Pointer](
		objc.ID(objc.GetClass("IOUSBHostDevice")),
		objc.RegisterName("createMatchingDictionaryWithVendorID:productID:bcdDevice:deviceClass:deviceSubclass:deviceProtocol:speed:productIDArray:"),
		nsNumberOrNil(vendorID), nsNumberOrNil(productID),
		unsafe.Pointer(nil), unsafe.Pointer(nil), unsafe.Pointer(nil),
		unsafe.Pointer(nil), unsafe.Pointer(nil), unsafe.Pointer(nil),
	)
}

// nsNumberOrNil returns an NSNumber for v, or nil for a negative value ("match
// any").
func nsNumberOrNil(v int) unsafe.Pointer {
	if v < 0 {
		return nil
	}
	return objc.SendClass("NSNumber", "numberWithInt:", int32(v))
}

// FindUSBAccessories returns the USB accessories currently connected to the host
// that match any of the given criteria. Passing no criteria matches any USB
// accessory.
//
// Using this requires the "com.apple.developer.accessory-access.usb" entitlement
// and presents a user-consent prompt, so it must run from an ordinary (Dock)
// application; it cannot be exercised headlessly.
//
// This is only supported on macOS 27 and newer, error will
// be returned on older versions.
func FindUSBAccessories(criteria ...*USBAccessoryMatchingCriteria) ([]*USBAccessory, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if err := loadAccessoryAccess(); err != nil {
		return nil, err
	}

	manager := objc.SendClass("AAUSBAccessoryManager", "sharedManager")
	listener := objc.NewObject(accessoryListenerGoClass)

	critObjs := make([]objc.NSObject, len(criteria))
	for i, c := range criteria {
		critObjs[i] = c
	}
	critArray := objc.ConvertToNSMutableArray(critObjs)

	type result struct {
		accessories []*USBAccessory
		err         error
	}
	ch := make(chan result, 1)
	block := objc.BlockObjectError(func(arrPtr, errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			ch <- result{err: err}
			return
		}
		ptrs := objc.NewNSArray(arrPtr).ToPointerSlice()
		accessories := make([]*USBAccessory, len(ptrs))
		for i, p := range ptrs {
			accessories[i] = newUSBAccessory(p)
		}
		ch <- result{accessories: accessories}
	})
	objc.SendVoid(manager, "registerListener:withMatchingCriteria:completionHandler:",
		listener, objc.Ptr(critArray), block)
	res := <-ch
	// registerListener may reference the criteria array asynchronously; keep the +1
	// array alive until the completion has fired (the receive above blocks until then).
	runtime.KeepAlive(critArray)
	block.Release()

	// Best-effort unregister so the process does not keep receiving events for a
	// one-shot lookup. The listener object and the no-op completion block are
	// intentionally not released (a rarely-called enumeration path).
	objc.SendVoid(manager, "unregisterListener:completionHandler:", listener, objc.BlockVoid(func() {}))

	return res.accessories, res.err
}
