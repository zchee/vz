package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestCustomVirtioDeviceRespondsToUpdateConfiguration verifies the selector
// UpdateDeviceSpecificConfiguration sends is spelled correctly and is available on
// VZCustomVirtioDevice.
//
// The runtime device is only vended to CustomVirtioHandler.DidCreateDevice once a
// virtual machine creates it, so the update firing itself is a manual e2e. This guard
// is headless: +[VZCustomVirtioDevice instancesRespondToSelector:] answers whether the
// concrete framework class implements the method without needing an instance, so a
// mistyped or unavailable selector fails here rather than silently at runtime.
func TestCustomVirtioDeviceRespondsToUpdateConfiguration(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	cls := objc.GetClass("VZCustomVirtioDevice")
	if objc.ID(cls) == 0 {
		t.Fatal("VZCustomVirtioDevice class did not resolve")
	}
	sel := objc.RegisterName("updateDeviceSpecificConfiguration:completionHandler:")
	if !objc.Send[bool](objc.ID(cls), objc.RegisterName("instancesRespondToSelector:"), sel) {
		t.Error("VZCustomVirtioDevice does not respond to updateDeviceSpecificConfiguration:completionHandler: — selector wrong or unavailable")
	}
}
