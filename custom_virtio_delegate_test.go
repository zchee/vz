package vz

import (
	"testing"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

func TestCustomVirtioDelegateClassesResolve(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	for _, name := range []string{"VZCustomVirtioDeviceProvider", "VZCustomVirtioDeviceDelegateProvider"} {
		if objc.ID(objc.GetClass(name)) == 0 {
			t.Errorf("%s class did not resolve", name)
		}
	}
	cls, err := ensureCustomVirtioDelegateClass()
	if err != nil {
		t.Fatalf("ensureCustomVirtioDelegateClass: %v", err)
	}
	if objc.ID(cls) == 0 {
		t.Fatal("Go-backed custom Virtio delegate class did not register")
	}
}

// TestCustomVirtioSetHandlerProviderAccepted resolves plan Open Q3: one Go-backed
// class implementing both the configuration- and device-delegate protocols is accepted
// by -[VZCustomVirtioDeviceDelegateProvider initWithDeviceQueue:delegate:] and attaches
// to the configuration.
func TestCustomVirtioSetHandlerProviderAccepted(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	config, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	config.SetHandler(CustomVirtioHandler{})
	if config.delegate == nil {
		t.Fatal("delegate object was not created")
	}
	if config.queue == nil {
		t.Fatal("device queue was not created")
	}
	if provider := objc.SendPtr(objc.Ptr(config), "provider"); provider == nil {
		t.Fatal("provider is nil: initWithDeviceQueue:delegate: or setProvider: failed")
	}
}

// TestCustomVirtioCallbackThread is the callback-thread plumbing probe. It confirms a
// Go IMP on the Go-backed delegate runs and recovers its associated state when invoked
// on the device's serial dispatch queue.
//
// Honest scope: this issues the call from a Go closure on the GCD queue, so it
// validates GCD-thread IMP execution and associated-state recovery — NOT the
// framework-C->Go transition with framework-vended pointer args. That foreign-caller
// transition is proven separately by internal/objc TestDefineClassKVOObserver and
// TestBlockForeignInvoke.
func TestCustomVirtioCallbackThread(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	config, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	fired := make(chan struct{}, 1)
	config.SetHandler(CustomVirtioHandler{
		DidAcceptDriverOK: func(*CustomVirtioDevice) { fired <- struct{}{} },
	})
	for i := range 100 {
		done := make(chan struct{})
		objc.DispatchAsync(config.queue, func() {
			objc.SendVoid(config.delegate, "customVirtioDeviceDidAcceptDriverOk:", unsafe.Pointer(nil))
			close(done)
		})
		<-done
		select {
		case <-fired:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: delegate IMP did not fire on the device queue", i)
		}
	}
}
