package vz

import (
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// CustomVirtioHandler holds the callbacks a host program provides to implement a
// custom Virtio device. Every callback is optional; set only the ones you need. The
// framework invokes them on the device's serial dispatch queue.
//
// The device and queue objects passed to the callbacks become fully functional once
// the framework creates the device and the guest driver reaches DRIVER_OK; see
// CustomVirtioDevice.
type CustomVirtioHandler struct {
	// DidCreateDevice is called when the framework creates the runtime device from
	// the configuration, during virtual machine construction.
	DidCreateDevice func(device *CustomVirtioDevice)
	// DidAcceptDriverOK is called when the guest driver sets DRIVER_OK; after this
	// the device's queues and negotiated features are valid.
	DidAcceptDriverOK func(device *CustomVirtioDevice)
	// DidReceiveNotificationForQueue is called when the guest kicks a virtqueue. The
	// queue and device are valid only for the duration of this callback (on the device
	// queue); do not retain them past it.
	DidReceiveNotificationForQueue func(device *CustomVirtioDevice, queue *VirtioQueue)
	// WillStop is called when the device stops (its virtual machine stopped).
	WillStop func(device *CustomVirtioDevice)
	// WillPause is called when the device pauses (its virtual machine paused).
	WillPause func(device *CustomVirtioDevice)
	// WillResume is called when the device resumes.
	WillResume func(device *CustomVirtioDevice)
	// WillReset is called when the device resets.
	WillReset func(device *CustomVirtioDevice)
	// SaveStateForRestore is called when the framework needs to save the device's
	// state so it can be restored later. Return the bytes to persist; return an empty
	// (or nil) slice if the device has no state to save — both are handed to the
	// framework as an empty NSData, which means "saved, no state". This is only invoked
	// when the configuration enabled SetSupportsSaveRestore.
	SaveStateForRestore func(device *CustomVirtioDevice) []byte
	// ShouldRestore is called when the framework restores the device from a previously
	// saved state — saveState is the slice a prior SaveStateForRestore returned. Return
	// true if the device restored successfully, false if the restore failed. This is
	// only invoked when the configuration enabled SetSupportsSaveRestore.
	ShouldRestore func(device *CustomVirtioDevice, saveState []byte) bool
}

// CustomVirtioDevice is the runtime custom Virtio device the framework creates from a
// CustomVirtioDeviceConfiguration, delivered to CustomVirtioHandler.DidCreateDevice.
// Its data-plane methods (queue access, guest-memory mapping) are provided by a later
// slice.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice?language=objc
type CustomVirtioDevice struct {
	*pointer

	// delegate and queue are the +1 references transferred from the configuration at
	// didCreateDevice: (see newCustomVirtioDevice). This wrapper releases them — and the
	// framework device — in its finalizer.
	delegate unsafe.Pointer
	queue    unsafe.Pointer
}

// VirtioQueue is a virtqueue (Virtio queue) belonging to a custom Virtio device. Its
// element-access methods are provided by a later slice.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueue?language=objc
type VirtioQueue struct {
	*pointer
}

// customVirtioState is the per-delegate Go state, associated with the Go-backed
// delegate object and recovered inside each IMP. The single Go-backed delegate class
// shares one IMP across all its instances, so per-instance state lives here rather
// than in an ivar.
type customVirtioState struct {
	handler CustomVirtioHandler
	config  *CustomVirtioDeviceConfiguration // back-ref, so didCreateDevice: can flip transferred
	device  *CustomVirtioDevice              // set at didCreateDevice: (device-queue only)
}

const customVirtioDelegateClassName = "VZCustomVirtioGoDelegate"

var (
	customVirtioDelegateOnce  sync.Once
	customVirtioDelegateClass objc.Class
	customVirtioDelegateErr   error
)

// ensureCustomVirtioDelegateClass registers, once, a single Go-backed Objective-C
// class conforming to BOTH VZCustomVirtioDeviceConfigurationDelegate (the config-time
// didCreateDevice: hook) and VZCustomVirtioDeviceDelegate (the runtime lifecycle and
// virtqueue-notification hooks). Objective-C does not enforce protocol conformance for
// id<Protocol> parameters at runtime, so one class implementing every method serves as
// both delegates.
func ensureCustomVirtioDelegateClass() (objc.Class, error) {
	customVirtioDelegateOnce.Do(func() {
		customVirtioDelegateClass, customVirtioDelegateErr = objc.DefineClass(
			customVirtioDelegateClassName,
			objc.NSObjectClass(),
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("customVirtioConfiguration:didCreateDevice:"), Fn: customVirtioDidCreateDevice},
				{Cmd: objc.RegisterName("customVirtioDevice:didReceiveNotificationForQueue:"), Fn: customVirtioDidReceiveNotification},
				{Cmd: objc.RegisterName("customVirtioDeviceDidAcceptDriverOk:"), Fn: customVirtioDidAcceptDriverOk},
				{Cmd: objc.RegisterName("customVirtioDeviceWillStop:"), Fn: customVirtioWillStop},
				{Cmd: objc.RegisterName("customVirtioDeviceWillPause:"), Fn: customVirtioWillPause},
				{Cmd: objc.RegisterName("customVirtioDeviceWillResume:"), Fn: customVirtioWillResume},
				{Cmd: objc.RegisterName("customVirtioDeviceWillReset:"), Fn: customVirtioWillReset},
				{Cmd: objc.RegisterName("customVirtioDeviceSaveStateForRestore:"), Fn: customVirtioSaveStateForRestore},
				{Cmd: objc.RegisterName("customVirtioDeviceShouldRestore:saveState:"), Fn: customVirtioShouldRestore},
			},
		)
	})
	return customVirtioDelegateClass, customVirtioDelegateErr
}

func customVirtioStateOf(self objc.ID) *customVirtioState {
	st, _ := objc.Associated(uintptr(self)).(*customVirtioState)
	return st
}

// The IMPs below recover the per-instance Go state and dispatch to the matching
// handler callback. The framework invokes them on the device's serial dispatch queue.
// The runtime device wrapper (st.device) is populated when the framework creates the
// device; its ownership handling is completed by a later slice.
func customVirtioDidCreateDevice(self objc.ID, _ objc.SEL, _, device unsafe.Pointer) {
	st := customVirtioStateOf(self)
	if st == nil {
		return
	}
	// Runs on the device queue. Take ownership of the +1 delegate + +1 queue from the
	// configuration onto the device wrapper (plan step 8), route the device's runtime
	// callbacks to our delegate, then hand the wrapper to the user.
	if st.device == nil && device != nil {
		st.device = newCustomVirtioDevice(device, st.config.delegate, st.config.queue, st.config)
		objc.SendVoid(device, "setDelegate:", st.config.delegate)
	}
	if st.handler.DidCreateDevice != nil {
		st.handler.DidCreateDevice(st.device)
	}
}

func customVirtioDidReceiveNotification(self objc.ID, _ objc.SEL, _, queue unsafe.Pointer) {
	st := customVirtioStateOf(self)
	if st == nil || st.handler.DidReceiveNotificationForQueue == nil {
		return
	}
	st.handler.DidReceiveNotificationForQueue(st.device, &VirtioQueue{pointer: objc.NewPointer(queue)})
}

func customVirtioDidAcceptDriverOk(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := customVirtioStateOf(self); st != nil && st.handler.DidAcceptDriverOK != nil {
		st.handler.DidAcceptDriverOK(st.device)
	}
}

func customVirtioWillStop(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := customVirtioStateOf(self); st != nil && st.handler.WillStop != nil {
		st.handler.WillStop(st.device)
	}
}

func customVirtioWillPause(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := customVirtioStateOf(self); st != nil && st.handler.WillPause != nil {
		st.handler.WillPause(st.device)
	}
}

func customVirtioWillResume(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := customVirtioStateOf(self); st != nil && st.handler.WillResume != nil {
		st.handler.WillResume(st.device)
	}
}

func customVirtioWillReset(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := customVirtioStateOf(self); st != nil && st.handler.WillReset != nil {
		st.handler.WillReset(st.device)
	}
}

// customVirtioSaveStateForRestore backs -[<delegate> customVirtioDeviceSaveStateForRestore:],
// a - (nullable NSData *) method the framework calls to snapshot the device (only when
// the configuration enabled SetSupportsSaveRestore). It runs on the device serial queue,
// after didCreateDevice:, so st.device is already set.
//
// It MUST return the NSData at +0: a - (NSData *) return does not transfer ownership, so
// the result is autoreleased (objc.AutoreleasedNSData). When no handler is set it returns
// an empty NSData, never a nil pointer — the framework treats a nil return as a failed
// save (see VZCustomVirtioDeviceDelegate.h), which is not what "no state" means. A nil or
// empty handler result is likewise handed over as an empty NSData. The body must not push
// its own autorelease pool: draining it would free the return before the framework copies
// it (the balancing drain is the device queue block's own GCD pool). The handler must not
// panic — a panic across the framework's C→Go call aborts the process, with no meaningful
// fallback return.
func customVirtioSaveStateForRestore(self objc.ID, _ objc.SEL, _ unsafe.Pointer) unsafe.Pointer {
	st := customVirtioStateOf(self)
	if st == nil || st.handler.SaveStateForRestore == nil {
		return objc.AutoreleasedNSData(nil)
	}
	return objc.AutoreleasedNSData(st.handler.SaveStateForRestore(st.device))
}

// customVirtioShouldRestore backs -[<delegate> customVirtioDeviceShouldRestore:saveState:],
// a - (BOOL) method the framework calls to restore the device from the bytes a prior
// customVirtioDeviceSaveStateForRestore: produced. It returns false (restore failed) when
// no handler is set.
//
// saveState is a +0 argument the framework owns; NSDataToBytes copies it into a Go slice
// (a nil NSData* becomes nil, a zero-length one becomes an empty slice), so this IMP must
// not release saveState and the handler receives a copy whose lifetime is independent of
// the framework's.
func customVirtioShouldRestore(self objc.ID, _ objc.SEL, _, saveState unsafe.Pointer) bool {
	st := customVirtioStateOf(self)
	if st == nil || st.handler.ShouldRestore == nil {
		return false
	}
	return st.handler.ShouldRestore(st.device, objc.NSDataToBytes(saveState))
}

// SetHandler makes this configuration emulate a custom Virtio device implemented in
// Go. It registers a Go-backed delegate, wraps it in a device delegate provider on its
// own serial dispatch queue, and attaches the provider to the configuration. Once the
// virtual machine starts, the framework invokes h's callbacks on that queue.
//
// Call SetHandler on a configuration obtained from NewCustomVirtioDeviceConfiguration
// (macOS 27+); calling it more than once replaces the provider with a new one.
func (c *CustomVirtioDeviceConfiguration) SetHandler(h CustomVirtioHandler) {
	cls, err := ensureCustomVirtioDelegateClass()
	if err != nil || objc.ID(cls) == 0 {
		return
	}
	// Release any delegate + queue a previous SetHandler installed — but only if the
	// configuration still owns them. After the didCreateDevice: transfer (transferred),
	// the device wrapper owns them and must not be double-freed here.
	if !c.transferred.Load() {
		if c.delegate != nil {
			objc.Disassociate(uintptr(c.delegate))
			objc.SendVoid(c.delegate, "release")
		}
		if c.queue != nil {
			objc.ReleaseDispatch(c.queue)
		}
	}
	delegate := objc.NewObject(cls) // +1
	queue := objc.DispatchQueueCreate(customVirtioDelegateClassName)
	objc.Associate(uintptr(delegate), &customVirtioState{handler: h, config: c})

	provider := objc.New(
		"VZCustomVirtioDeviceDelegateProvider",
		"initWithDeviceQueue:delegate:",
		queue, delegate,
	) // +1
	objc.SendVoid(objc.Ptr(c), "setProvider:", provider)
	objc.SendVoid(provider, "release") // the config's provider property (strong) keeps its own reference

	// The configuration owns the delegate (+1) and queue (+1); its finalizer releases
	// them until the didCreateDevice: transfer moves ownership to the runtime
	// *CustomVirtioDevice wrapper and sets transferred (plan step 8). Reset transferred so
	// the configuration owns these new references.
	c.delegate = delegate
	c.queue = queue
	c.transferred.Store(false)
}
