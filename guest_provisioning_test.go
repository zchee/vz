//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestMacGuestProvisioningOptions verifies the property setters round-trip
// through the Objective-C object on a macOS 27 host.
func TestMacGuestProvisioningOptions(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("guest provisioning requires macOS 27+: %v", err)
	}

	opts, err := NewMacGuestProvisioningOptions()
	if err != nil {
		t.Fatalf("NewMacGuestProvisioningOptions: %v", err)
	}
	opts.SetFullName("Test User")
	opts.SetUsername("tester")
	opts.SetPassword("secret")
	opts.SetLogsInAutomatically(true)
	opts.SetEnablesRemoteLogin(true)

	getString := func(sel string) string {
		return objc.GoString(objc.SendPtr(objc.SendPtr(objc.Ptr(opts), sel), "UTF8String"))
	}
	getBool := func(sel string) bool {
		return objc.Send[bool](objc.ID(uintptr(objc.Ptr(opts))), objc.RegisterName(sel))
	}

	if got := getString("fullName"); got != "Test User" {
		t.Errorf("fullName = %q, want %q", got, "Test User")
	}
	if got := getString("username"); got != "tester" {
		t.Errorf("username = %q, want %q", got, "tester")
	}
	if !getBool("logsInAutomatically") {
		t.Error("logsInAutomatically = false, want true")
	}
	if !getBool("enablesRemoteLogin") {
		t.Error("enablesRemoteLogin = false, want true")
	}
}

// TestStartOptionsShareSingleObject guards the interaction between the two macOS
// start options: applying both must configure one shared start-options object
// carrying both settings, rather than the second option clobbering the first.
func TestStartOptionsShareSingleObject(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("guest provisioning requires macOS 27+: %v", err)
	}

	opts, err := NewMacGuestProvisioningOptions()
	if err != nil {
		t.Fatalf("NewMacGuestProvisioningOptions: %v", err)
	}
	opts.SetFullName("Test User")
	opts.SetUsername("tester")
	opts.SetPassword("secret")

	sopt := &virtualMachineStartOptions{}
	if err := WithGuestProvisioningOptions(opts)(sopt); err != nil {
		t.Fatalf("WithGuestProvisioningOptions: %v", err)
	}
	first := sopt.macOSVirtualMachineStartOptionsPtr
	if first == nil {
		t.Fatal("start options pointer is nil after WithGuestProvisioningOptions")
	}

	if err := WithStartUpFromMacOSRecovery(true)(sopt); err != nil {
		t.Fatalf("WithStartUpFromMacOSRecovery: %v", err)
	}
	if sopt.macOSVirtualMachineStartOptionsPtr != first {
		t.Fatal("start options object was replaced; both options must share one object")
	}

	id := objc.ID(uintptr(sopt.macOSVirtualMachineStartOptionsPtr))
	if !objc.Send[bool](id, objc.RegisterName("startUpFromMacOSRecovery")) {
		t.Error("startUpFromMacOSRecovery = false, want true")
	}
	if objc.SendPtr(sopt.macOSVirtualMachineStartOptionsPtr, "guestProvisioningOptions") == nil {
		t.Error("guestProvisioningOptions is nil, want the options we set")
	}
}
