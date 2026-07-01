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
	// DidReceiveNotificationForQueue is called when the guest kicks a virtqueue.
	DidReceiveNotificationForQueue func(device *CustomVirtioDevice, queue *VirtioQueue)
	// WillStop is called when the device stops (its virtual machine stopped).
	WillStop func(device *CustomVirtioDevice)
	// WillPause is called when the device pauses (its virtual machine paused).
	WillPause func(device *CustomVirtioDevice)
	// WillResume is called when the device resumes.
	WillResume func(device *CustomVirtioDevice)
	// WillReset is called when the device resets.
	WillReset func(device *CustomVirtioDevice)
}

// CustomVirtioDevice is the runtime custom Virtio device the framework creates from a
// CustomVirtioDeviceConfiguration, delivered to CustomVirtioHandler.DidCreateDevice.
// Its data-plane methods (queue access, guest-memory mapping) are provided by a later
// slice.
//
// see: https://developer.apple.com/documentation/virtualization/vzcustomvirtiodevice?language=objc
type CustomVirtioDevice struct {
	*pointer
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
	device  *CustomVirtioDevice // nil in B2; B3 sets it in the didCreateDevice transfer (device-queue only)
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
	// B2 stub: this slice never starts a VM, so the framework never calls this. B3
	// completes ownership here — retaining the framework device, re-anchoring the +1
	// delegate/queue to this wrapper, and setting transferred=true — and must confine
	// st.device access to the device queue (the config finalizer runs off-queue).
	if st.device == nil && device != nil {
		st.device = &CustomVirtioDevice{pointer: objc.NewPointer(device)}
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
	// Replace any delegate + queue a previous SetHandler installed.
	if c.delegate != nil {
		objc.Disassociate(uintptr(c.delegate))
		objc.SendVoid(c.delegate, "release")
	}
	if c.queue != nil {
		objc.ReleaseDispatch(c.queue)
	}
	delegate := objc.NewObject(cls) // +1
	queue := objc.DispatchQueueCreate(customVirtioDelegateClassName)
	objc.Associate(uintptr(delegate), &customVirtioState{handler: h})

	provider := objc.New(
		"VZCustomVirtioDeviceDelegateProvider",
		"initWithDeviceQueue:delegate:",
		queue, delegate,
	) // +1
	objc.SendVoid(objc.Ptr(c), "setProvider:", provider)
	objc.SendVoid(provider, "release") // the config's provider property (strong) keeps its own reference

	// Own the delegate (+1) and queue (+1) on the configuration; the finalizer releases
	// them. This is a B2 scaffold: because no VM starts here, config-anchoring is safe,
	// but B3 MUST re-anchor them to the VM-run *CustomVirtioDevice wrapper and set
	// transferred=true before any VM starts (see the field doc + plan step 8), otherwise
	// dropping the config wrapper frees the delegate mid-run.
	c.delegate = delegate
	c.queue = queue
}
