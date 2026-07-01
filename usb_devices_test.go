package vz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestXHCIControllerSetUSBDevices exercises the config-time USB device list.
// Compile-time conformance of *USBMassStorageDeviceConfiguration to
// USBDeviceConfiguration is asserted in storage.go; this verifies the
// setUsbDevices: selector and the usbDevices getter round-trip at runtime.
func TestXHCIControllerSetUSBDevices(t *testing.T) {
	if err := macOSAvailable(15); err != nil {
		t.Skipf("config-time USB device list requires macOS 15+: %v", err)
	}

	config, err := NewXHCIControllerConfiguration()
	if err != nil {
		t.Fatalf("NewXHCIControllerConfiguration: %v", err)
	}

	usbDeviceCount := func() int {
		arr := objc.NewNSArray(objc.SendPtr(objc.Ptr(config), "usbDevices"))
		return len(arr.ToPointerSlice())
	}

	// An empty list must round-trip through setUsbDevices: (proves the selector
	// name; a wrong selector raises doesNotRecognizeSelector).
	config.SetUSBDevices(nil)
	if got := usbDeviceCount(); got != 0 {
		t.Fatalf("empty SetUSBDevices: usbDevices count = %d, want 0", got)
	}

	// One mass-storage device backed by a temporary raw disk image.
	diskPath := filepath.Join(t.TempDir(), "disk.img")
	f, err := os.Create(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	attachment, err := NewDiskImageStorageDeviceAttachment(diskPath, true)
	if err != nil {
		t.Fatalf("NewDiskImageStorageDeviceAttachment: %v", err)
	}
	massStorage, err := NewUSBMassStorageDeviceConfiguration(attachment)
	if err != nil {
		t.Fatalf("NewUSBMassStorageDeviceConfiguration: %v", err)
	}

	config.SetUSBDevices([]USBDeviceConfiguration{massStorage})
	if got := usbDeviceCount(); got != 1 {
		t.Fatalf("SetUSBDevices with one device: usbDevices count = %d, want 1", got)
	}
}
