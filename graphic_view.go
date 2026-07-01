package vz

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// This file and graphic_view_delegate.go implement StartGraphicApplication with
// purego instead of cgo. They drive an AppKit application (window, toolbar, menu
// bar, zoom/scroll) that displays a running virtual machine. The AppKit
// frameworks are loaded lazily on first use so headless VMs never touch AppKit.

// AppKit / Foundation enumeration constants used by the graphical application.
// These are compile-time NS_ENUM/NS_OPTIONS values (not exported symbols), so
// they are reproduced here with their documented macOS values.
const (
	nsWindowStyleMaskTitled         = 1 << 0
	nsWindowStyleMaskClosable       = 1 << 1
	nsWindowStyleMaskMiniaturizable = 1 << 2
	nsWindowStyleMaskResizable      = 1 << 3

	nsBackingStoreBuffered = 2
	nsWindowTitleHidden    = 1

	nsEventMaskAny         = ^uint64(0) // NSEventMaskAny (NSUIntegerMax)
	nsEventMaskMouseMoved  = 1 << 5     // 1 << NSEventTypeMouseMoved
	nsEventMaskScrollWheel = 1 << 22    // 1 << NSEventTypeScrollWheel

	nsEventModifierFlagCommand = 1 << 20
	nsEventModifierFlagOption  = 1 << 19

	nsVisualEffectBlendingModeWithinWindow = 1
	nsVisualEffectStateActive              = 1

	nsViewWidthSizable  = 2
	nsViewHeightSizable = 16

	nsControlStateValueOn  = 1
	nsControlStateValueOff = 0

	nsApplicationActivationPolicyRegular = 0

	nsAlertStyleWarning      = 0
	nsAlertStyleCritical     = 2
	nsAlertFirstButtonReturn = 1000

	nsBezelStyleTexturedRounded = 11
	nsButtonTypeToggle          = 2

	nsToolbarDisplayModeIconOnly = 2

	nsTextAlignmentCenter  = 2 // macOS value (TARGET_ABI_USES_IOS_VALUES is 0)
	nsUnderlineStyleSingle = 1

	nsLayoutConstraintOrientationHorizontal   = 0
	nsLayoutConstraintOrientationVertical     = 1
	nsUserInterfaceLayoutOrientationVertical  = 1
	nsStackViewDistributionFillProportionally = 2
	nsLayoutAttributeCenterX                  = 9

	// VZVirtualMachineState values mirrored for the GUI's KVO handler.
	vmStateGUIStopped = int(VirtualMachineStateStopped)
	vmStateGUIError   = int(VirtualMachineStateError)
	vmStateGUIPaused  = int(VirtualMachineStatePaused)
)

// nsLayoutPriorityRequired is NSLayoutPriorityRequired (a float, 1000).
const nsLayoutPriorityRequired float64 = 1000

const cocoaFrameworkPath = "/System/Library/Frameworks/Cocoa.framework/Cocoa"

// Lazily-initialised GUI runtime state: the two Go-defined Objective-C classes
// and the extern NSString constants that AppKit exposes as data symbols.
var (
	guiOnce sync.Once
	guiErr  error

	appDelegateGoClass objc.Class

	// Cached extern NSString constants (data symbols).
	nsDefaultRunLoopMode         unsafe.Pointer
	nsToolbarSpaceItemID         unsafe.Pointer
	nsToolbarFlexibleSpaceItemID unsafe.Pointer
	nsForegroundColorAttrName    unsafe.Pointer
	nsFontAttrName               unsafe.Pointer
	nsLinkAttrName               unsafe.Pointer
	nsUnderlineStyleAttrName     unsafe.Pointer
	nsImageNameCaution           unsafe.Pointer

	// appShouldKeepRunning drives VZApplication's manual run loop.
	appShouldKeepRunning atomic.Bool
)

// initGUI performs one-time AppKit setup: it loads Cocoa, registers the
// VZApplication and AppDelegate classes, and resolves the extern NSString
// constants. It is safe to call repeatedly.
func initGUI() error {
	guiOnce.Do(func() {
		if err := objc.LoadFramework(cocoaFrameworkPath); err != nil {
			guiErr = err
			return
		}

		nsDefaultRunLoopMode = objc.ExternPointer("NSDefaultRunLoopMode")
		nsToolbarSpaceItemID = objc.ExternPointer("NSToolbarSpaceItemIdentifier")
		nsToolbarFlexibleSpaceItemID = objc.ExternPointer("NSToolbarFlexibleSpaceItemIdentifier")
		nsForegroundColorAttrName = objc.ExternPointer("NSForegroundColorAttributeName")
		nsFontAttrName = objc.ExternPointer("NSFontAttributeName")
		nsLinkAttrName = objc.ExternPointer("NSLinkAttributeName")
		nsUnderlineStyleAttrName = objc.ExternPointer("NSUnderlineStyleAttributeName")
		nsImageNameCaution = objc.ExternPointer("NSImageNameCaution")

		// VZApplication is looked up by name via -sharedApplication, so the
		// returned class value is not retained here.
		if _, err := objc.DefineClass(
			"VZApplicationGo",
			objc.GetClass("NSApplication"),
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("run"), Fn: vzAppRun},
				{Cmd: objc.RegisterName("terminate:"), Fn: vzAppTerminate},
			},
		); err != nil {
			guiErr = err
			return
		}

		appDelegateGoClass, guiErr = defineAppDelegateClass()
	})
	return guiErr
}

// vzAppRun implements -[VZApplication run]: a manual event loop that can be
// stopped by -terminate: so StartGraphicApplication returns to Go instead of
// the process exiting. The whole loop runs under one autorelease pool, matching
// the original @autoreleasepool.
func vzAppRun(self objc.ID, _ objc.SEL) {
	pool := objc.AutoreleasePoolPush()
	defer objc.AutoreleasePoolPop(pool)

	self.Send(objc.RegisterName("finishLaunching"))
	appShouldKeepRunning.Store(true)

	distantFuture := objc.SendClass("NSDate", "distantFuture")
	nextEvent := objc.RegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	sendEvent := objc.RegisterName("sendEvent:")
	updateWindows := objc.RegisterName("updateWindows")
	for appShouldKeepRunning.Load() {
		event := objc.Send[unsafe.Pointer](self, nextEvent,
			nsEventMaskAny, distantFuture, nsDefaultRunLoopMode, true)
		self.Send(sendEvent, event)
		self.Send(updateWindows)
	}
}

// vzAppTerminate implements -[VZApplication terminate:]: it ends the manual run
// loop and posts an event so a loop currently blocked in nextEventMatchingMask
// wakes up.
func vzAppTerminate(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	appShouldKeepRunning.Store(false)
	current := objc.Send[unsafe.Pointer](self, objc.RegisterName("currentEvent"))
	self.Send(objc.RegisterName("postEvent:atStart:"), current, false)
}

// --- small message-send helpers to keep the AppKit call sites readable ---

func nsStr(s string) unsafe.Pointer { return objc.NSString(s) }

// msg sends sel to obj and returns an object pointer.
func msg(obj unsafe.Pointer, sel string, args ...any) unsafe.Pointer {
	return objc.SendPtr(obj, sel, args...)
}

// msgv sends sel to obj discarding the result.
func msgv(obj unsafe.Pointer, sel string, args ...any) {
	objc.SendVoid(obj, sel, args...)
}

// msgClass sends sel to the named class and returns an object pointer.
func msgClass(class, sel string, args ...any) unsafe.Pointer {
	return objc.SendClass(class, sel, args...)
}

// newObj allocates and initialises an instance of class via initSel.
func newObj(class, initSel string, args ...any) unsafe.Pointer {
	return objc.New(class, initSel, args...)
}

// makeNSArray builds an NSArray from object pointers.
func makeNSArray(items ...unsafe.Pointer) unsafe.Pointer {
	arr := msgClass("NSMutableArray", "array")
	for _, it := range items {
		msgv(arr, "addObject:", it)
	}
	return arr
}

type startGraphicApplicationOptions struct {
	title            string
	enableController bool
}

// StartGraphicApplicationOption is an option for display graphics start.
type StartGraphicApplicationOption func(*startGraphicApplicationOptions) error

// WithWindowTitle is an option to set window title of display graphics window.
func WithWindowTitle(title string) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.title = title
		return nil
	}
}

// WithController is an option to set virtual machine controller on graphics window toolbar.
func WithController(enable bool) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.enableController = enable
		return nil
	}
}

// StartGraphicApplication starts an application to display graphics of the VM.
//
// You must to call runtime.LockOSThread before calling this method.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) StartGraphicApplication(width, height float64, opts ...StartGraphicApplicationOption) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	if err := initGUI(); err != nil {
		return err
	}

	defaultOpts := &startGraphicApplicationOptions{}
	for _, opt := range opts {
		if err := opt(defaultOpts); err != nil {
			return err
		}
	}

	// Create the shared application instance (a VZApplication subclass), so NSApp
	// is initialised with the custom run loop before the delegate is built.
	app := msgClass("VZApplicationGo", "sharedApplication")

	win := newGraphicWindow(v, app, width, height, defaultOpts)

	msgv(app, "setDelegate:", win.delegate)
	msgv(app, "run")

	win.teardown()
	return nil
}
