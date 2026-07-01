package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestAccessoryAccessLoads verifies AccessoryAccess.framework and
// IOUSBHost.framework load and their classes resolve. FindUSBAccessories itself
// is not exercised: it requires the com.apple.developer.accessory-access.usb
// entitlement and presents a user-consent prompt, so it can only be validated
// manually in a signed Dock app with a physical device.
func TestAccessoryAccessLoads(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("AccessoryAccess requires macOS 27+: %v", err)
	}
	if err := loadAccessoryAccess(); err != nil {
		t.Fatalf("loadAccessoryAccess: %v", err)
	}
	for _, name := range []string{"AAUSBAccessoryManager", "AAUSBAccessoryMatchingCriteria", "IOUSBHostDevice"} {
		if objc.ID(objc.GetClass(name)) == 0 {
			t.Errorf("%s class did not resolve after loading its framework", name)
		}
	}
}

// TestUSBMatchingDictionary verifies the IOKit USB device matching dictionary
// that criteria creation depends on. This step needs no entitlement, so it is
// verified fully on the macOS 27 host.
func TestUSBMatchingDictionary(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("AccessoryAccess requires macOS 27+: %v", err)
	}
	if err := loadAccessoryAccess(); err != nil {
		t.Fatalf("loadAccessoryAccess: %v", err)
	}

	dict := usbMatchingDictionary(0x05ac, 0x1234)
	if dict == nil {
		t.Fatal("usbMatchingDictionary returned nil")
	}
	defer objc.SendVoid(dict, "release")

	id := objc.ID(uintptr(dict))
	if !objc.Send[bool](id, objc.RegisterName("isKindOfClass:"), objc.GetClass("NSDictionary")) {
		t.Fatal("matching dictionary is not an NSDictionary")
	}

	intForKey := func(key string) int {
		v := objc.SendPtr(dict, "objectForKey:", objc.NSString(key))
		if v == nil {
			return -1
		}
		return objc.Send[int](objc.ID(uintptr(v)), objc.RegisterName("intValue"))
	}
	if got := intForKey("idVendor"); got != 0x05ac {
		t.Errorf("idVendor = %d, want %d", got, 0x05ac)
	}
	if got := intForKey("idProduct"); got != 0x1234 {
		t.Errorf("idProduct = %d, want %d", got, 0x1234)
	}
}

// TestNewUSBAccessoryMatchingCriteria verifies criteria creation is graceful. In
// an unsigned CLI test binary (no accessory-access entitlement, not a Dock app)
// AccessoryAccess rejects the criteria, so this must return an error rather than
// panic; with the entitlement it returns a non-nil criteria.
func TestNewUSBAccessoryMatchingCriteria(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("AccessoryAccess requires macOS 27+: %v", err)
	}

	criteria, err := NewUSBAccessoryMatchingCriteria(0x05ac, 0x1234)
	if err != nil {
		t.Logf("criteria creation returned an error (expected without the accessory-access entitlement): %v", err)
		return
	}
	if objc.Ptr(criteria) == nil {
		t.Fatal("criteria creation succeeded but the pointer is nil")
	}
}
