package vz

import (
	"runtime"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestCustomVirtioRuntimeClassesResolve verifies the runtime data-plane classes resolve
// on a macOS 27 host.
func TestCustomVirtioRuntimeClassesResolve(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	for _, name := range []string{
		"VZVirtioQueue", "VZVirtioQueueElement", "VZGuestMemoryMapping", "VZNegotiatedVirtioFeatureSet",
	} {
		if objc.ID(objc.GetClass(name)) == 0 {
			t.Errorf("%s class did not resolve", name)
		}
	}
}

// TestCustomVirtioOwnershipTransfer exercises the load-bearing plan step-8 transfer
// headlessly: newCustomVirtioDevice takes over the +1 delegate and +1 queue from the
// configuration and sets transferred, so dropping the configuration afterwards no longer
// tears the delegate down. A stand-in NSObject stands in for the framework device
// (newCustomVirtioDevice only retains/releases and wraps whatever it is given); the
// setDelegate: routing that needs a live VM is exercised in the manual e2e test.
func TestCustomVirtioOwnershipTransfer(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	config, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	config.SetHandler(CustomVirtioHandler{})
	if config.transferred.Load() {
		t.Fatal("transferred should be false before the transfer")
	}
	delegate := config.delegate
	queue := config.queue
	if delegate == nil || queue == nil {
		t.Fatal("SetHandler did not install a delegate/queue")
	}

	// Stand-in for the framework device; balance our own +1 after the wrapper takes its.
	standIn := objc.NewObject(objc.GetClass("NSObject"))
	dev := newCustomVirtioDevice(standIn, config.delegate, config.queue, config)
	objc.SendVoid(standIn, "release")

	if !config.transferred.Load() {
		t.Fatal("transferred should be true after newCustomVirtioDevice")
	}
	if dev.delegate != delegate || dev.queue != queue {
		t.Fatal("the device wrapper did not take the delegate/queue")
	}
	if objc.Associated(uintptr(delegate)) == nil {
		t.Fatal("delegate state was disassociated by the transfer")
	}

	// Drop the configuration. With transferred set, its finalizer must NOT disassociate
	// or release the delegate — the device wrapper owns it now. Had the transfer failed
	// (transferred false), the config finalizer would disassociate the delegate.
	config = nil
	runtime.GC()
	runtime.GC()
	if objc.Associated(uintptr(delegate)) == nil {
		t.Fatal("delegate was torn down after dropping the config — the ownership transfer did not protect it")
	}
	runtime.KeepAlive(dev)
}

// TestCustomVirtioSetHandlerAfterTransfer verifies SetHandler is safe to call after the
// transfer: it must not double-free the delegate/queue now owned by the device wrapper,
// and it re-takes ownership of a fresh delegate/queue (transferred reset to false).
func TestCustomVirtioSetHandlerAfterTransfer(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	config, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	config.SetHandler(CustomVirtioHandler{})
	first := config.delegate
	firstQueue := config.queue

	// Simulate the transfer having happened (the device wrapper now owns first + queue).
	config.transferred.Store(true)

	// A second SetHandler must not release the transferred delegate/queue, must install a
	// new pair, and must reset transferred.
	config.SetHandler(CustomVirtioHandler{})
	if config.transferred.Load() {
		t.Fatal("transferred should be reset to false after SetHandler installs a new delegate/queue")
	}
	if config.delegate == first {
		t.Fatal("SetHandler did not install a new delegate")
	}
	if config.delegate == nil || config.queue == nil {
		t.Fatal("SetHandler did not install a delegate/queue")
	}

	// first + firstQueue are still owned by the (simulated) device wrapper; release them
	// here so the test itself stays leak-free.
	objc.Disassociate(uintptr(first))
	objc.SendVoid(first, "release")
	objc.ReleaseDispatch(firstQueue)
}
