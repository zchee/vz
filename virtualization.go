package vz

import (
	"fmt"
	"sync"
	"unsafe"

	infinity "github.com/Code-Hex/go-infinity-channel"
	"github.com/Code-Hex/vz/v3/internal/objc"
	"github.com/Code-Hex/vz/v3/internal/sliceutil"
)

// VirtualMachineState represents execution state of the virtual machine.
//
//go:generate stringer -type=VirtualMachineState
type VirtualMachineState int

const (
	// VirtualMachineStateStopped Initial state before the virtual machine is started.
	VirtualMachineStateStopped VirtualMachineState = iota

	// VirtualMachineStateRunning Running virtual machine.
	VirtualMachineStateRunning

	// VirtualMachineStatePaused A started virtual machine is paused.
	// This state can only be transitioned from VirtualMachineStatePausing.
	VirtualMachineStatePaused

	// VirtualMachineStateError The virtual machine has encountered an internal error.
	VirtualMachineStateError

	// VirtualMachineStateStarting The virtual machine is configuring the hardware and starting.
	VirtualMachineStateStarting

	// VirtualMachineStatePausing The virtual machine is being paused.
	// This is the intermediate state between VirtualMachineStateRunning and VirtualMachineStatePaused.
	VirtualMachineStatePausing

	// VirtualMachineStateResuming The virtual machine is being resumed.
	// This is the intermediate state between VirtualMachineStatePaused and VirtualMachineStateRunning.
	VirtualMachineStateResuming

	// VirtualMachineStateStopping The virtual machine is being stopped.
	// This is the intermediate state between VirtualMachineStateRunning and VirtualMachineStateStop.
	//
	// Available on macOS 12.0 and above.
	VirtualMachineStateStopping

	// VirtualMachineStateSaving The virtual machine is being saved.
	// This is the intermediate state between VirtualMachineStatePaused and VirtualMachineStatePaused
	//
	// Available on macOS 14.0 and above.
	VirtualMachineStateSaving

	// VirtualMachineStateRestoring	The virtual machine is being restored.
	// This is the intermediate state between VirtualMachineStateStopped and either VirtualMachineStatePaused on success or VirtualMachineStateStopped on failure.
	//
	// Available on macOS 14.0 and above.
	VirtualMachineStateRestoring
)

// nsKeyValueObservingOptionNew mirrors NSKeyValueObservingOptionNew.
const nsKeyValueObservingOptionNew = 1 << 0

// nsKeyValueObservingOptionInitial mirrors NSKeyValueObservingOptionInitial.
const nsKeyValueObservingOptionInitial = 1 << 2

// nsNotFound mirrors Foundation's NSNotFound (NSIntegerMax).
const nsNotFound = int(^uint(0) >> 1)

// VirtualMachine represents the entire state of a single virtual machine.
//
// A Virtual Machine is the emulation of a complete hardware machine of the same architecture as the real hardware machine.
// When executing the Virtual Machine, the Virtualization framework uses certain hardware resources and emulates others to provide isolation
// and great performance.
//
// The definition of a virtual machine starts with its configuration. This is done by setting up a VirtualMachineConfiguration struct.
// Once configured, the virtual machine can be started with (*VirtualMachine).Start() method.
//
// Creating a virtual machine using the Virtualization framework requires the app to have the "com.apple.security.virtualization" entitlement.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine?language=objc
type VirtualMachine struct {
	// id for this struct.
	id string

	// Indicate whether or not virtualization is available.
	//
	// If virtualization is unavailable, no VirtualMachineConfiguration will validate.
	// The validation error of the VirtualMachineConfiguration provides more information about why virtualization is unavailable.
	supported bool

	*pointer
	dispatchQueue unsafe.Pointer
	machineState  *machineState

	// stateObserver is the KVO observer watching the "state" property, and
	// networkDelegate is the VM delegate receiving network-disconnect events.
	// The Virtualization framework retains neither (KVO does not retain its
	// observer, and VZVirtualMachine.delegate is a weak reference), so they are
	// held here for the VM's lifetime and torn down in finalize.
	stateObserver   unsafe.Pointer
	networkDelegate unsafe.Pointer

	disconnectedIn        *infinity.Channel[*disconnected]
	disconnectedOut       *infinity.Channel[*DisconnectedError]
	watchDisconnectedOnce sync.Once

	finalizeOnce sync.Once

	config *VirtualMachineConfiguration

	mu sync.RWMutex
}

type machineState struct {
	state       VirtualMachineState
	stateNotify *infinity.Channel[VirtualMachineState]

	mu sync.RWMutex
}

// vmStateObserver is the Go-defined Objective-C class that observes a
// VZVirtualMachine's "state" property via KVO. Its single method publishes each
// new state to the machineState associated with the observer instance.
var (
	vmStateObserverClass objc.Class
	vmStateObserverOnce  sync.Once
)

func vmStateObserver() objc.Class {
	vmStateObserverOnce.Do(func() {
		cls, err := objc.DefineClass(
			"VZVirtualMachineStateObserverGo",
			objc.NSObjectClass(),
			[]objc.MethodDef{{
				Cmd: objc.RegisterName("observeValueForKeyPath:ofObject:change:context:"),
				Fn:  vmStateObserve,
			}},
		)
		if err != nil {
			panic("vz: failed to define VM state observer: " + err.Error())
		}
		vmStateObserverClass = cls
	})
	return vmStateObserverClass
}

// vmStateObserve implements
// -[observer observeValueForKeyPath:ofObject:change:context:]. It reads the new
// VZVirtualMachineState from the KVO change dictionary and publishes it.
func vmStateObserve(self objc.ID, _ objc.SEL, _, _, change, _ unsafe.Pointer) {
	ms, ok := objc.Associated(uintptr(self)).(*machineState)
	if !ok {
		return
	}
	// change[NSKeyValueChangeNewKey] is the boxed new "state" value.
	newValue := objc.SendPtr(change, "objectForKey:", objc.NSString("new"))
	newState := VirtualMachineState(
		objc.Send[int](objc.ID(uintptr(newValue)), objc.RegisterName("integerValue")),
	)
	ms.mu.Lock()
	ms.state = newState
	ms.stateNotify.In() <- newState
	ms.mu.Unlock()
}

// vmNetworkDelegate is the Go-defined Objective-C class set as the
// VZVirtualMachine delegate. It forwards network-attachment disconnect events to
// the disconnected channel associated with the delegate instance.
var (
	vmNetworkDelegateClass objc.Class
	vmNetworkDelegateOnce  sync.Once
)

func vmNetworkDelegate() objc.Class {
	vmNetworkDelegateOnce.Do(func() {
		cls, err := objc.DefineClass(
			"VZVirtualMachineNetworkDelegateGo",
			objc.NSObjectClass(),
			[]objc.MethodDef{{
				Cmd: objc.RegisterName("virtualMachine:networkDevice:attachmentWasDisconnectedWithError:"),
				Fn:  vmNetworkAttachmentDisconnected,
			}},
		)
		if err != nil {
			panic("vz: failed to define VM network delegate: " + err.Error())
		}
		vmNetworkDelegateClass = cls
	})
	return vmNetworkDelegateClass
}

// vmNetworkAttachmentDisconnected implements
// -[delegate virtualMachine:networkDevice:attachmentWasDisconnectedWithError:].
// It resolves the index of the disconnected network device within the VM's
// networkDevices and forwards the event with that index.
func vmNetworkAttachmentDisconnected(self objc.ID, _ objc.SEL, vm, networkDevice, errPtr unsafe.Pointer) {
	ch, ok := objc.Associated(uintptr(self)).(*infinity.Channel[*disconnected])
	if !ok {
		return
	}
	devices := objc.SendPtr(vm, "networkDevices")
	index := objc.Send[int](
		objc.ID(uintptr(devices)),
		objc.RegisterName("indexOfObject:"),
		networkDevice,
	)
	if index == nsNotFound {
		index = -1
	}
	ch.In() <- &disconnected{
		err:   newNSError(errPtr),
		index: index,
	}
}

// NewVirtualMachine creates a new VirtualMachine with VirtualMachineConfiguration.
//
// The configuration must be valid. Validation can be performed at runtime with (*VirtualMachineConfiguration).Validate() method.
// The configuration is copied by the initializer.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtualMachine(config *VirtualMachineConfiguration) (*VirtualMachine, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	label := objc.GoString(objc.GetUUID())
	dispatchQueue := objc.DispatchQueueCreate(label)

	machineState := &machineState{
		state:       VirtualMachineState(0),
		stateNotify: infinity.NewChannel[VirtualMachineState](),
	}

	disconnectedIn := infinity.NewChannel[*disconnected]()
	disconnectedOut := infinity.NewChannel[*DisconnectedError]()

	vmPtr := objc.New(
		"VZVirtualMachine", "initWithConfiguration:queue:",
		objc.Ptr(config),
		dispatchQueue,
	)

	// Observe "state" so VM transitions are published to machineState.
	stateObserver := objc.NewObject(vmStateObserver())
	objc.Associate(uintptr(stateObserver), machineState)
	objc.SendVoid(
		vmPtr, "addObserver:forKeyPath:options:context:",
		stateObserver,
		objc.NSString("state"),
		uint(nsKeyValueObservingOptionNew),
		unsafe.Pointer(nil),
	)

	// Install the network-disconnect delegate. delegate is a weak property, so
	// networkDelegate must outlive the VM; it is released in finalize.
	networkDelegate := objc.NewObject(vmNetworkDelegate())
	objc.Associate(uintptr(networkDelegate), disconnectedIn)
	objc.SendVoid(vmPtr, "setDelegate:", networkDelegate)

	v := &VirtualMachine{
		id:              label,
		pointer:         objc.NewPointer(vmPtr),
		dispatchQueue:   dispatchQueue,
		machineState:    machineState,
		stateObserver:   stateObserver,
		networkDelegate: networkDelegate,
		disconnectedIn:  disconnectedIn,
		disconnectedOut: disconnectedOut,
		config:          config,
	}

	objc.SetFinalizer(v, func(self *VirtualMachine) {
		self.finalize()
	})
	return v, nil
}

func (v *VirtualMachine) finalize() {
	v.finalizeOnce.Do(func() {
		// KVO requires removing the observer before the observed VM deallocates.
		objc.SendVoid(objc.Ptr(v), "removeObserver:forKeyPath:",
			v.stateObserver, objc.NSString("state"))
		objc.ReleaseDispatch(v.dispatchQueue)
		objc.Release(v)
		// The observer and delegate are not retained by the framework, so they
		// are released here now that the VM is gone, and their associated Go
		// state is dropped.
		objc.SendVoid(v.stateObserver, "release")
		objc.SendVoid(v.networkDelegate, "release")
		objc.Disassociate(uintptr(v.stateObserver))
		objc.Disassociate(uintptr(v.networkDelegate))
	})
}

// SocketDevices return the list of socket devices configured on this virtual machine.
// Return an empty array if no socket device is configured.
//
// Since only NewVirtioSocketDeviceConfiguration is available in vz package,
// it will always return VirtioSocketDevice.
// see: https://developer.apple.com/documentation/virtualization/vzvirtualmachine/3656702-socketdevices?language=objc
func (v *VirtualMachine) SocketDevices() []*VirtioSocketDevice {
	nsArray := objc.NewNSArray(
		objc.SendPtr(objc.Ptr(v), "socketDevices"),
	)
	ptrs := nsArray.ToPointerSlice()
	socketDevices := make([]*VirtioSocketDevice, len(ptrs))
	for i, ptr := range ptrs {
		socketDevices[i] = newVirtioSocketDevice(ptr, v.dispatchQueue)
	}
	return socketDevices
}

// USBControllers return the list of USB controllers configured on this virtual machine. Return an empty array if no USB controller is configured.
//
// This is only supported on macOS 15 and newer, nil will
// be returned on older versions.
func (v *VirtualMachine) USBControllers() []*USBController {
	if err := macOSAvailable(15); err != nil {
		return nil
	}
	nsArray := objc.NewNSArray(
		objc.SendPtr(objc.Ptr(v), "usbControllers"),
	)
	ptrs := nsArray.ToPointerSlice()
	usbControllers := make([]*USBController, len(ptrs))
	for i, ptr := range ptrs {
		usbControllers[i] = newUSBController(ptr, v.dispatchQueue)
	}
	return usbControllers
}

// State represents execution state of the virtual machine.
func (v *VirtualMachine) State() VirtualMachineState {
	v.machineState.mu.RLock()
	defer v.machineState.mu.RUnlock()
	return v.machineState.state
}

// StateChangedNotify gets notification is changed execution state of the virtual machine.
func (v *VirtualMachine) StateChangedNotify() <-chan VirtualMachineState {
	v.machineState.mu.RLock()
	defer v.machineState.mu.RUnlock()
	return v.machineState.stateNotify.Out()
}

// canDispatch reads a boolean VZVirtualMachine property on the VM's serial queue.
func (v *VirtualMachine) canDispatch(sel string) bool {
	var ret bool
	objc.DispatchSync(v.dispatchQueue, func() {
		ret = objc.Send[bool](objc.ID(uintptr(objc.Ptr(v))), objc.RegisterName(sel))
	})
	return ret
}

// CanStart returns true if the machine is in a state that can be started.
func (v *VirtualMachine) CanStart() bool { return v.canDispatch("canStart") }

// CanPause returns true if the machine is in a state that can be paused.
func (v *VirtualMachine) CanPause() bool { return v.canDispatch("canPause") }

// CanResume returns true if the machine is in a state that can be resumed.
func (v *VirtualMachine) CanResume() bool { return v.canDispatch("canResume") }

// CanRequestStop returns whether the machine is in a state where the guest can be asked to stop.
func (v *VirtualMachine) CanRequestStop() bool { return v.canDispatch("canRequestStop") }

// CanStop returns whether the machine is in a state that can be stopped.
//
// This is only supported on macOS 12 and newer, false will always be returned
// on older versions.
func (v *VirtualMachine) CanStop() bool {
	if err := macOSAvailable(12); err != nil {
		return false
	}
	return v.canDispatch("canStop")
}

func makeHandler() (func(error), chan error) {
	ch := make(chan error, 1)
	return func(err error) {
		ch <- err
		close(ch)
	}, ch
}

// completionBlockError builds a void(^)(NSError *) completion block that
// forwards the framework's error (or nil) to ch exactly once.
func completionBlockError(ch chan error) objc.Block {
	return objc.BlockError(func(errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			ch <- err
		} else {
			ch <- nil
		}
	})
}

type virtualMachineStartOptions struct {
	macOSVirtualMachineStartOptionsPtr unsafe.Pointer
}

// VirtualMachineStartOption is an option for virtual machine start.
type VirtualMachineStartOption func(*virtualMachineStartOptions) error

// Start a virtual machine that is in either Stopped or Error state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// If options are specified, also checks whether these options are
// available in use your macOS version available.
func (v *VirtualMachine) Start(opts ...VirtualMachineStartOption) error {
	o := &virtualMachineStartOptions{}
	for _, optFunc := range opts {
		if err := optFunc(o); err != nil {
			return err
		}
	}

	errCh := make(chan error, 1)
	block := completionBlockError(errCh)
	objc.DispatchSync(v.dispatchQueue, func() {
		if o.macOSVirtualMachineStartOptionsPtr != nil {
			objc.SendVoid(objc.Ptr(v), "startWithOptions:completionHandler:",
				o.macOSVirtualMachineStartOptionsPtr, block)
		} else {
			objc.SendVoid(objc.Ptr(v), "startWithCompletionHandler:", block)
		}
	})
	err := <-errCh
	block.Release()
	return err
}

// lifecycle issues a control selector taking a single completionHandler: on the
// VM's queue and waits for the completion handler to fire. The completion block
// is released only after the result has been received; the framework retains its
// own copy across the asynchronous completion.
func (v *VirtualMachine) lifecycle(sel string) error {
	errCh := make(chan error, 1)
	block := completionBlockError(errCh)
	objc.DispatchSync(v.dispatchQueue, func() {
		objc.SendVoid(objc.Ptr(v), sel, block)
	})
	err := <-errCh
	block.Release()
	return err
}

// Pause a virtual machine that is in Running state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Pause() error {
	return v.lifecycle("pauseWithCompletionHandler:")
}

// Resume a virtual machine that is in the Paused state.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
func (v *VirtualMachine) Resume() error {
	return v.lifecycle("resumeWithCompletionHandler:")
}

// RequestStop requests that the guest turns itself off.
//
// If returned error is not nil, assigned with the error if the request failed.
// Returns true if the request was made successfully.
func (v *VirtualMachine) RequestStop() (bool, error) {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	var ret bool
	objc.DispatchSync(v.dispatchQueue, func() {
		ret = objc.Send[bool](
			objc.ID(uintptr(objc.Ptr(v))),
			objc.RegisterName("requestStopWithError:"),
			errSlot,
		)
	})
	if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
		return ret, err
	}
	return ret, nil
}

// Stop stops a VM that’s in either a running or paused state.
//
// The completion handler returns an error object when the VM fails to stop,
// or nil if the stop was successful.
//
// If you want to listen status change events, use the "StateChangedNotify" method.
//
// Warning: This is a destructive operation. It stops the VM without
// giving the guest a chance to stop cleanly.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) Stop() error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	return v.lifecycle("stopWithCompletionHandler:")
}

// DisconnectedError represents an error that occurs when a VM’s network attachment is disconnected
// due to a network-related issue. This error is triggered by the framework when such a disconnection happens.
type DisconnectedError struct {
	// Err is the underlying error that caused the disconnection, triggered by the framework.
	// This error provides information on why the network attachment was disconnected.
	Err error
	// The network device configuration associated with the disconnection event.
	// This configuration helps identify which network device experienced the disconnection.
	// If Config is nil, the specific configuration details are unavailable.
	Config *VirtioNetworkDeviceConfiguration
}

var _ error = (*DisconnectedError)(nil)

func (e *DisconnectedError) Unwrap() error { return e.Err }
func (e *DisconnectedError) Error() string {
	if e.Config == nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Config.attachment, e.Err)
}

type disconnected struct {
	err   error
	index int
}

// NetworkDeviceAttachmentWasDisconnected returns a receive channel.
// The channel emits an error message each time the network attachment is disconnected,
// typically triggered by events such as failure to start, initial boot, device reset, or reboot.
// As a result, this method may be invoked multiple times throughout the virtual machine's lifecycle.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) NetworkDeviceAttachmentWasDisconnected() (<-chan *DisconnectedError, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	v.watchDisconnectedOnce.Do(func() {
		go v.watchDisconnected()
	})
	return v.disconnectedOut.Out(), nil
}

// TODO(codehex): refactoring to leave using machineState's mutex lock.
func (v *VirtualMachine) watchDisconnected() {
	for disconnected := range v.disconnectedIn.Out() {
		v.mu.RLock()
		config := sliceutil.FindValueByIndex(
			v.config.networkDeviceConfiguration,
			disconnected.index,
		)
		v.mu.RUnlock()
		v.disconnectedOut.In() <- &DisconnectedError{
			Err:    disconnected.err,
			Config: config,
		}
	}
	v.disconnectedOut.Close()
}
