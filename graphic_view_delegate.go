package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// Toolbar item identifiers used by the graphical application.
const (
	idZoom   = "Zoom"
	idPause  = "Pause"
	idPlay   = "Play"
	idPower  = "Power"
	idSpace  = "Space"
	idSpace2 = "Space2"
)

// graphicWindow holds the per-application GUI state associated with the
// AppDelegate instance. All mutation happens on the main thread (AppKit event
// handlers and main-queue callbacks), so no locking is needed.
type graphicWindow struct {
	vm       unsafe.Pointer // VZVirtualMachine*
	queue    unsafe.Pointer // dispatch_queue_t
	app      unsafe.Pointer // VZApplication*
	delegate unsafe.Pointer // AppDelegateGo*
	view     unsafe.Pointer // VZVirtualMachineView*
	window   unsafe.Pointer // NSWindow*
	toolbar  unsafe.Pointer // NSToolbar*

	pauseOverlay unsafe.Pointer // NSVisualEffectView*

	enableController bool
	isZoomEnabled    bool

	scrollTimer        unsafe.Pointer
	scrollDelta        objc.Point
	mouseMovedMonitor  unsafe.Pointer
	scrollWheelMonitor unsafe.Pointer

	// Event-monitor blocks kept alive for the app's lifetime; released in teardown.
	mouseBlock  objc.Block
	scrollBlock objc.Block
}

// windowFor returns the graphicWindow associated with an AppDelegate instance.
func windowFor(self objc.ID) *graphicWindow {
	st, _ := objc.Associated(uintptr(self)).(*graphicWindow)
	return st
}

// defineAppDelegateClass registers the AppDelegate Objective-C class. It acts as
// the NSApplicationDelegate, NSWindowDelegate and NSToolbarDelegate, and as a KVO
// observer of the VM's "state". It is intentionally NOT the VZVirtualMachine
// delegate: state-driven UI and termination are handled through KVO, leaving the
// VM's own network-disconnect delegate in place.
func defineAppDelegateClass() (objc.Class, error) {
	return objc.DefineClass(
		"VZGraphicAppDelegateGo",
		objc.NSObjectClass(),
		[]objc.MethodDef{
			{Cmd: objc.RegisterName("observeValueForKeyPath:ofObject:change:context:"), Fn: appObserve},
			{Cmd: objc.RegisterName("applicationDidFinishLaunching:"), Fn: appDidFinishLaunching},
			{Cmd: objc.RegisterName("windowWillClose:"), Fn: appWindowWillClose},
			{Cmd: objc.RegisterName("toolbar:itemForItemIdentifier:willBeInsertedIntoToolbar:"), Fn: appToolbarItemFor},
			{Cmd: objc.RegisterName("toolbarDefaultItemIdentifiers:"), Fn: appToolbarDefaultIDs},
			{Cmd: objc.RegisterName("toolbarAllowedItemIdentifiers:"), Fn: appToolbarAllowedIDs},
			{Cmd: objc.RegisterName("pauseButtonClicked:"), Fn: appPauseClicked},
			{Cmd: objc.RegisterName("powerButtonClicked:"), Fn: appPowerClicked},
			{Cmd: objc.RegisterName("playButtonClicked:"), Fn: appPlayClicked},
			{Cmd: objc.RegisterName("toggleZoomMode:"), Fn: appToggleZoom},
			{Cmd: objc.RegisterName("handleMagnification:"), Fn: appHandleMagnification},
			{Cmd: objc.RegisterName("scrollTick:"), Fn: appScrollTick},
			{Cmd: objc.RegisterName("toggleCapturesSystemKeys:"), Fn: appToggleCaptures},
			{Cmd: objc.RegisterName("reportIssue:"), Fn: appReportIssue},
			{Cmd: objc.RegisterName("openAboutWindow:"), Fn: appOpenAbout},
		},
	)
}

// --- IMP shims: thin adapters from (self, _cmd, args) to *graphicWindow methods.

func appObserve(self objc.ID, _ objc.SEL, _, _, change, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.observeState(change)
	}
}

func appDidFinishLaunching(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.applicationDidFinishLaunching()
	}
}

func appWindowWillClose(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		msgv(st.app, "terminate:", unsafe.Pointer(nil))
	}
}

func appToolbarItemFor(self objc.ID, _ objc.SEL, _, itemID unsafe.Pointer, _ bool) unsafe.Pointer {
	st := windowFor(self)
	if st == nil {
		return nil
	}
	return st.toolbarItemFor(itemID)
}

func appToolbarDefaultIDs(self objc.ID, _ objc.SEL, _ unsafe.Pointer) unsafe.Pointer {
	st := windowFor(self)
	if st == nil {
		return nil
	}
	return makeNSArray(st.toolbarItemIdentifiers()...)
}

func appToolbarAllowedIDs(self objc.ID, _ objc.SEL, _ unsafe.Pointer) unsafe.Pointer {
	st := windowFor(self)
	if st == nil {
		return nil
	}
	return makeNSArray(
		nsStr(idZoom), nsStr(idPlay), nsStr(idPause), nsStr(idSpace),
		nsStr(idSpace2), nsStr(idPower), nsToolbarSpaceItemID, nsToolbarFlexibleSpaceItemID,
	)
}

func appPauseClicked(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.pauseClicked()
	}
}

func appPowerClicked(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.powerClicked()
	}
}

func appPlayClicked(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.playClicked()
	}
}

func appToggleZoom(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.toggleZoom()
	}
}

func appHandleMagnification(self objc.ID, _ objc.SEL, recognizer unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.handleMagnification(recognizer)
	}
}

func appScrollTick(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.scrollTick()
	}
}

func appToggleCaptures(self objc.ID, _ objc.SEL, sender unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.toggleCapturesSystemKeys(sender)
	}
}

func appReportIssue(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.reportIssue()
	}
}

func appOpenAbout(self objc.ID, _ objc.SEL, _ unsafe.Pointer) {
	if st := windowFor(self); st != nil {
		st.openAboutWindow()
	}
}

// newGraphicWindow builds the AppDelegate and its backing state, mirroring the
// original -initWithVirtualMachine:… : it creates the VM view, main window and
// toolbar, installs the GUI's KVO observer, and adds the pause overlay.
func newGraphicWindow(v *VirtualMachine, app unsafe.Pointer, width, height float64, opts *startGraphicApplicationOptions) *graphicWindow {
	delegate := objc.NewObject(appDelegateGoClass)
	st := &graphicWindow{
		vm:               objc.Ptr(v),
		queue:            v.dispatchQueue,
		app:              app,
		delegate:         delegate,
		enableController: opts.enableController,
	}
	objc.Associate(uintptr(delegate), st)

	view := newObj("VZVirtualMachineView", "init")
	msgv(view, "setCapturesSystemKeys:", true)
	msgv(view, "setVirtualMachine:", st.vm)
	if macOSAvailable(14) == nil {
		msgv(view, "setAutomaticallyReconfiguresDisplay:", true)
	}
	st.view = view

	st.window = st.createMainWindow(opts.title, width, height)
	st.toolbar = st.createCustomToolbar()

	msgv(st.vm, "addObserver:forKeyPath:options:context:",
		delegate, nsStr("state"), uint(nsKeyValueObservingOptionNew), unsafe.Pointer(nil))

	st.pauseOverlay = st.createPauseOverlay(view)
	msgv(view, "addSubview:", st.pauseOverlay)
	return st
}

// teardown removes the observers and monitors and releases the event-monitor
// blocks after the run loop exits.
func (st *graphicWindow) teardown() {
	if st.mouseMovedMonitor != nil {
		msgClass("NSEvent", "removeMonitor:", st.mouseMovedMonitor)
		st.mouseBlock.Release()
	}
	if st.scrollWheelMonitor != nil {
		msgClass("NSEvent", "removeMonitor:", st.scrollWheelMonitor)
		st.scrollBlock.Release()
	}
	st.stopScrollTimer()
	msgv(st.vm, "removeObserver:forKeyPath:", st.delegate, nsStr("state"))
	objc.Disassociate(uintptr(st.delegate))
}

// --- KVO ---

func (st *graphicWindow) observeState(change unsafe.Pointer) {
	newValue := msg(change, "objectForKey:", nsStr("new"))
	newState := objc.Send[int](objc.ID(uintptr(newValue)), objc.RegisterName("integerValue"))
	objc.DispatchAsyncMain(func() {
		st.updateToolbarItems()
		if newState == vmStateGUIPaused {
			st.showOverlay()
		} else {
			st.hideOverlay()
		}
		// Terminating the GUI application from guest or host: the original also
		// terminated via -virtualMachine:didStopWithError:, which drives state to
		// Error, so handling Stopped or Error here covers both paths.
		if newState == vmStateGUIStopped || newState == vmStateGUIError {
			msgv(st.app, "terminate:", unsafe.Pointer(nil))
		}
	})
}

func (st *graphicWindow) showOverlay() {
	if st.pauseOverlay != nil {
		msgv(st.pauseOverlay, "setHidden:", false)
	}
}

func (st *graphicWindow) hideOverlay() {
	if st.pauseOverlay != nil {
		msgv(st.pauseOverlay, "setHidden:", true)
	}
}

// --- application / window setup ---

func (st *graphicWindow) applicationDidFinishLaunching() {
	st.setupMenuBar()
	st.setupGraphicWindow()
	// Required so the menu bar activates even though the app is launched
	// programmatically rather than from a bundle.
	msgv(st.app, "setActivationPolicy:", int(nsApplicationActivationPolicyRegular))
	msgv(st.app, "activateIgnoringOtherApps:", true)
}

func (st *graphicWindow) createMainWindow(title string, width, height float64) unsafe.Pointer {
	rect := objc.Rect{Size: objc.Size{Width: width, Height: height}}
	styleMask := uint(nsWindowStyleMaskTitled | nsWindowStyleMaskClosable |
		nsWindowStyleMaskMiniaturizable | nsWindowStyleMaskResizable)
	window := newObj("NSWindow", "initWithContentRect:styleMask:backing:defer:",
		rect, styleMask, uint(nsBackingStoreBuffered), false)
	msgv(window, "setTitle:", nsStr(title))
	return window
}

func (st *graphicWindow) createCustomToolbar() unsafe.Pointer {
	toolbar := newObj("NSToolbar", "initWithIdentifier:", nsStr("CustomToolbar"))
	msgv(toolbar, "setDelegate:", st.delegate)
	msgv(toolbar, "setDisplayMode:", uint(nsToolbarDisplayModeIconOnly))
	msgv(toolbar, "setShowsBaselineSeparator:", false)
	msgv(toolbar, "setAllowsUserCustomization:", false)
	msgv(toolbar, "setAutosavesConfiguration:", false)
	return toolbar
}

func (st *graphicWindow) createPauseOverlay(view unsafe.Pointer) unsafe.Pointer {
	bounds := objc.Send[objc.Rect](objc.ID(uintptr(view)), objc.RegisterName("bounds"))
	effectView := newObj("NSVisualEffectView", "initWithFrame:", bounds)
	msgv(effectView, "setWantsLayer:", true)
	msgv(effectView, "setBlendingMode:", uint(nsVisualEffectBlendingModeWithinWindow))
	msgv(effectView, "setState:", uint(nsVisualEffectStateActive))
	msgv(effectView, "setAlphaValue:", float64(0.7))
	msgv(effectView, "setAutoresizingMask:", uint(nsViewWidthSizable|nsViewHeightSizable))
	msgv(effectView, "setHidden:", true)
	return effectView
}

func (st *graphicWindow) setupGraphicWindow() {
	msgv(st.window, "setTitlebarAppearsTransparent:", true)
	msgv(st.window, "setToolbar:", st.toolbar)
	msgv(st.window, "setOpaque:", false)
	msgv(st.window, "center")

	// Event monitors driving zoom auto-scroll behaviour. The blocks return the
	// event to keep it flowing through the responder chain.
	st.mouseBlock = objc.BlockEventMonitor(func(event unsafe.Pointer) unsafe.Pointer {
		st.handleMouseMovement(event)
		return event
	})
	st.mouseMovedMonitor = msgClass("NSEvent", "addLocalMonitorForEventsMatchingMask:handler:",
		uint64(nsEventMaskMouseMoved), st.mouseBlock)

	st.scrollBlock = objc.BlockEventMonitor(func(event unsafe.Pointer) unsafe.Pointer {
		st.handleScrollWheel(event)
		return event
	})
	st.scrollWheelMonitor = msgClass("NSEvent", "addLocalMonitorForEventsMatchingMask:handler:",
		uint64(nsEventMaskScrollWheel), st.scrollBlock)

	scrollView := st.createScrollView(st.view)
	msgv(st.window, "setContentView:", scrollView)

	msgv(st.view, "setTranslatesAutoresizingMaskIntoConstraints:", false)
	contentView := msg(st.window, "contentView")
	activateEdgeConstraints(st.view, contentView)

	sizeInPixels := st.virtualMachineSizeInPixels()
	if sizeInPixels.Width != 0 || sizeInPixels.Height != 0 {
		msgv(st.window, "setContentAspectRatio:", sizeInPixels)
		frame := objc.Send[objc.Rect](objc.ID(uintptr(st.window)), objc.RegisterName("frame"))
		w := frame.Size.Width
		h := w * (sizeInPixels.Height / sizeInPixels.Width)
		msgv(st.window, "setContentSize:", objc.Size{Width: w, Height: h})
	}

	msgv(st.window, "setDelegate:", st.delegate)
	msgv(st.window, "makeKeyAndOrderFront:", unsafe.Pointer(nil))
	// Prevents a crash on window close (releasedWhenClosed defaults to YES for
	// windows without a controller).
	msgv(st.window, "setReleasedWhenClosed:", false)
}

func (st *graphicWindow) virtualMachineSizeInPixels() objc.Size {
	var size objc.Size
	if macOSAvailable(14) != nil {
		return size
	}
	objc.DispatchSync(st.queue, func() {
		devices := msg(st.vm, "graphicsDevices")
		if objc.Send[uint64](objc.ID(uintptr(devices)), objc.RegisterName("count")) == 0 {
			return
		}
		gd := msg(devices, "objectAtIndex:", uint64(0))
		displays := msg(gd, "displays")
		if objc.Send[uint64](objc.ID(uintptr(displays)), objc.RegisterName("count")) == 0 {
			return
		}
		disp := msg(displays, "objectAtIndex:", uint64(0))
		size = objc.Send[objc.Size](objc.ID(uintptr(disp)), objc.RegisterName("sizeInPixels"))
	})
	return size
}

func (st *graphicWindow) createScrollView(view unsafe.Pointer) unsafe.Pointer {
	contentView := msg(st.window, "contentView")
	bounds := objc.Send[objc.Rect](objc.ID(uintptr(contentView)), objc.RegisterName("bounds"))
	scrollView := newObj("NSScrollView", "initWithFrame:", bounds)
	msgv(scrollView, "setHasVerticalScroller:", false)
	msgv(scrollView, "setHasHorizontalScroller:", false)
	msgv(scrollView, "setAutohidesScrollers:", true)
	msgv(scrollView, "setAutoresizingMask:", uint(nsViewWidthSizable|nsViewHeightSizable))
	msgv(scrollView, "setDocumentView:", view)
	msgv(scrollView, "setAllowsMagnification:", true)
	msgv(scrollView, "setMaxMagnification:", float64(4.0))
	msgv(scrollView, "setMinMagnification:", float64(1.0))

	recognizer := msg(msgClass("NSMagnificationGestureRecognizer", "alloc"),
		"initWithTarget:action:", st.delegate, objc.RegisterName("handleMagnification:"))
	// Immediately propagate pinch gestures to the VZVirtualMachineView.
	msgv(recognizer, "setDelaysMagnificationEvents:", false)
	msgv(scrollView, "addGestureRecognizer:", recognizer)
	return scrollView
}

// activateEdgeConstraints pins view to contentView on all four edges.
func activateEdgeConstraints(view, contentView unsafe.Pointer) {
	anchors := [...]string{"leadingAnchor", "trailingAnchor", "topAnchor", "bottomAnchor"}
	constraints := make([]unsafe.Pointer, 0, len(anchors))
	for _, a := range anchors {
		constraints = append(constraints, msg(msg(view, a), "constraintEqualToAnchor:", msg(contentView, a)))
	}
	msgClass("NSLayoutConstraint", "activateConstraints:", makeNSArray(constraints...))
}

// --- toolbar ---

func (st *graphicWindow) toolbarItemIdentifiers() []unsafe.Pointer {
	items := make([]unsafe.Pointer, 0, 8)
	if st.enableController {
		if st.vmCan("canPause") {
			items = append(items, nsStr(idPause))
		}
		if st.vmCan("canResume") {
			items = append(items, nsStr(idSpace), nsStr(idPlay))
		}
		if st.vmCan("canStop") || st.vmCan("canStart") {
			items = append(items, nsStr(idSpace2), nsStr(idPower))
		}
	}
	items = append(items, nsToolbarSpaceItemID, nsStr(idZoom), nsToolbarFlexibleSpaceItemID)
	return items
}

func (st *graphicWindow) updateToolbarItems() {
	st.setToolbarItems(st.toolbarItemIdentifiers())
}

func (st *graphicWindow) setToolbarItems(desired []unsafe.Pointer) {
	if st.toolbar == nil {
		return
	}
	for objc.Send[uint64](objc.ID(uintptr(msg(st.toolbar, "items"))), objc.RegisterName("count")) > 0 {
		msgv(st.toolbar, "removeItemAtIndex:", uint64(0))
	}
	for _, id := range desired {
		count := objc.Send[uint64](objc.ID(uintptr(msg(st.toolbar, "items"))), objc.RegisterName("count"))
		msgv(st.toolbar, "insertItemWithItemIdentifier:atIndex:", id, count)
	}
}

func (st *graphicWindow) toolbarItemFor(itemID unsafe.Pointer) unsafe.Pointer {
	item := newObj("NSToolbarItem", "initWithItemIdentifier:", itemID)
	switch objc.GoString(msg(itemID, "UTF8String")) {
	case idPause:
		msgv(item, "setImage:", symbolImage("pause.fill"))
		msgv(item, "setLabel:", nsStr("Pause"))
		msgv(item, "setTarget:", st.delegate)
		msgv(item, "setToolTip:", nsStr("Pause"))
		msgv(item, "setBordered:", true)
		msgv(item, "setAction:", objc.RegisterName("pauseButtonClicked:"))
	case idPower:
		msgv(item, "setImage:", symbolImage("power"))
		msgv(item, "setLabel:", nsStr("Power"))
		msgv(item, "setTarget:", st.delegate)
		msgv(item, "setToolTip:", nsStr("Power ON/OFF"))
		msgv(item, "setBordered:", true)
		msgv(item, "setAction:", objc.RegisterName("powerButtonClicked:"))
	case idPlay:
		msgv(item, "setImage:", symbolImage("play.fill"))
		msgv(item, "setLabel:", nsStr("Play"))
		msgv(item, "setTarget:", st.delegate)
		msgv(item, "setToolTip:", nsStr("Resume"))
		msgv(item, "setBordered:", true)
		msgv(item, "setAction:", objc.RegisterName("playButtonClicked:"))
	case idZoom:
		zoomButton := newObj("NSButton", "initWithFrame:", objc.Rect{Size: objc.Size{Width: 40, Height: 40}})
		msgv(zoomButton, "setBezelStyle:", uint(nsBezelStyleTexturedRounded))
		msgv(zoomButton, "setImage:", symbolImage("plus.magnifyingglass"))
		msgv(zoomButton, "setTarget:", st.delegate)
		msgv(zoomButton, "setAction:", objc.RegisterName("toggleZoomMode:"))
		msgv(zoomButton, "setButtonType:", uint(nsButtonTypeToggle))
		msgv(item, "setView:", zoomButton)
		msgv(item, "setLabel:", nsStr("Zoom"))
		msgv(item, "setToolTip:", nsStr("Toggle Zoom"))
	case idSpace, idSpace2:
		spaceView := newObj("NSView", "initWithFrame:", objc.Rect{Size: objc.Size{Width: 2, Height: 10}})
		msgv(item, "setView:", spaceView)
		msgv(item, "setMinSize:", objc.Size{Width: 1, Height: 10})
		msgv(item, "setMaxSize:", objc.Size{Width: 1, Height: 10})
	}
	return item
}

// --- VM controls ---

func (st *graphicWindow) vmCan(sel string) bool {
	var ret bool
	objc.DispatchSync(st.queue, func() {
		ret = objc.Send[bool](objc.ID(uintptr(st.vm)), objc.RegisterName(sel))
	})
	return ret
}

// vmControlOnQueue issues a completion-handler VM control selector on the VM's
// queue and shows an alert on failure. The caller must already be on the main
// thread.
func (st *graphicWindow) vmControlOnQueue(sel, failMsg string) {
	objc.DispatchSync(st.queue, func() {
		block := objc.BlockError(func(errPtr unsafe.Pointer) {
			if errPtr != nil {
				st.showErrorAlert(failMsg, errPtr)
			}
		})
		msgv(st.vm, sel, block)
	})
}

func (st *graphicWindow) pauseClicked() {
	objc.DispatchAsyncMain(func() {
		st.vmControlOnQueue("pauseWithCompletionHandler:", "Failed to pause Virtual Machine")
	})
}

func (st *graphicWindow) playClicked() {
	objc.DispatchAsyncMain(func() {
		st.vmControlOnQueue("resumeWithCompletionHandler:", "Failed to resume Virtual Machine")
	})
}

func (st *graphicWindow) powerClicked() {
	objc.DispatchAsyncMain(func() {
		if st.vmCan("canStart") {
			st.vmControlOnQueue("startWithCompletionHandler:", "Failed to start Virtual Machine")
			return
		}
		if st.vmCan("canStop") {
			alert := newObj("NSAlert", "init")
			msgv(alert, "setIcon:", msgClass("NSImage", "imageNamed:", nsImageNameCaution))
			msgv(alert, "setMessageText:", nsStr("Force Stop Warning"))
			msgv(alert, "setInformativeText:", nsStr("This action will stop the VM without a clean shutdown, similar to unplugging a PC.\n\nDo you want to force stop?"))
			msgv(alert, "setAlertStyle:", uint(nsAlertStyleWarning))
			msgv(alert, "addButtonWithTitle:", nsStr("Stop"))
			msgv(alert, "addButtonWithTitle:", nsStr("Cancel"))
			response := objc.Send[int](objc.ID(uintptr(alert)), objc.RegisterName("runModal"))
			if response != nsAlertFirstButtonReturn {
				return
			}
			st.vmControlOnQueue("stopWithCompletionHandler:", "Failed to stop Virtual Machine")
		}
	})
}

// showErrorAlert reads the error synchronously (it is only valid during the
// completion handler) and presents the alert on the main thread.
func (st *graphicWindow) showErrorAlert(message string, errPtr unsafe.Pointer) {
	desc := objc.ErrorLocalizedDescription(errPtr)
	code := objc.ErrorCode(errPtr)
	objc.DispatchAsyncMain(func() {
		alert := newObj("NSAlert", "init")
		msgv(alert, "setMessageText:", nsStr(message))
		msgv(alert, "setInformativeText:", nsStr(fmt.Sprintf("Error: %s\nCode: %d", desc, code)))
		msgv(alert, "setAlertStyle:", uint(nsAlertStyleCritical))
		msgv(alert, "addButtonWithTitle:", nsStr("OK"))
		msgv(alert, "runModal")
	})
}

// --- zoom / scroll ---

func (st *graphicWindow) toggleZoom() {
	st.isZoomEnabled = !st.isZoomEnabled
	scrollView := msg(st.window, "contentView")
	if !st.isZoomEnabled {
		animBlock := objc.BlockObject(func(ctx unsafe.Pointer) {
			msgv(ctx, "setDuration:", float64(0.3))
			msgv(msg(scrollView, "animator"), "setMagnification:", float64(1.0))
		})
		completionBlock := objc.BlockVoid(func() {
			if isKindOf(scrollView, "NSScrollView") {
				msgv(scrollView, "setHasVerticalScroller:", false)
				msgv(scrollView, "setHasHorizontalScroller:", false)
			}
		})
		msgClass("NSAnimationContext", "runAnimationGroup:completionHandler:", animBlock, completionBlock)
		return
	}
	if isKindOf(scrollView, "NSScrollView") {
		msgv(scrollView, "setHasVerticalScroller:", true)
		msgv(scrollView, "setHasHorizontalScroller:", true)
		msgv(scrollView, "setAutohidesScrollers:", true)
	}
}

func (st *graphicWindow) handleMagnification(recognizer unsafe.Pointer) {
	if !st.isZoomEnabled {
		return
	}
	scrollView := msg(recognizer, "view")
	svID := objc.ID(uintptr(scrollView))
	mag := objc.Send[float64](svID, objc.RegisterName("magnification"))
	recogMag := objc.Send[float64](objc.ID(uintptr(recognizer)), objc.RegisterName("magnification"))
	maxMag := objc.Send[float64](svID, objc.RegisterName("maxMagnification"))
	minMag := objc.Send[float64](svID, objc.RegisterName("minMagnification"))
	newMag := min(maxMag, max(minMag, mag+recogMag))

	locInView := objc.Send[objc.Point](objc.ID(uintptr(recognizer)), objc.RegisterName("locationInView:"), scrollView)
	clipView := msg(scrollView, "contentView")
	centered := objc.Send[objc.Point](objc.ID(uintptr(clipView)), objc.RegisterName("convertPoint:fromView:"), locInView, scrollView)
	msgv(scrollView, "setMagnification:centeredAtPoint:", newMag, centered)
}

func (st *graphicWindow) handleScrollWheel(event unsafe.Pointer) {
	if !st.isZoomEnabled {
		return
	}
	mods := objc.Send[uint64](objc.ID(uintptr(event)), objc.RegisterName("modifierFlags"))
	if mods&nsEventModifierFlagCommand == 0 && mods&nsEventModifierFlagOption == 0 {
		return
	}
	scrollView := msg(st.window, "contentView")
	if !isKindOf(scrollView, "NSScrollView") {
		return
	}
	svID := objc.ID(uintptr(scrollView))
	deltaY := objc.Send[float64](objc.ID(uintptr(event)), objc.RegisterName("scrollingDeltaY"))
	cur := objc.Send[float64](svID, objc.RegisterName("magnification"))
	maxMag := objc.Send[float64](svID, objc.RegisterName("maxMagnification"))
	minMag := objc.Send[float64](svID, objc.RegisterName("minMagnification"))
	newMag := min(maxMag, max(minMag, cur+deltaY*0.01))

	contentView := msg(st.window, "contentView")
	locInWindow := objc.Send[objc.Point](objc.ID(uintptr(event)), objc.RegisterName("locationInWindow"))
	mouseLoc := objc.Send[objc.Point](objc.ID(uintptr(contentView)), objc.RegisterName("convertPoint:fromView:"), locInWindow, unsafe.Pointer(nil))
	clipView := msg(scrollView, "contentView")
	centered := objc.Send[objc.Point](objc.ID(uintptr(clipView)), objc.RegisterName("convertPoint:fromView:"), mouseLoc, contentView)
	msgv(scrollView, "setMagnification:centeredAtPoint:", newMag, centered)
}

func (st *graphicWindow) handleMouseMovement(event unsafe.Pointer) {
	if !st.isZoomEnabled {
		st.stopScrollTimer()
		return
	}
	scrollView := msg(st.window, "contentView")
	if !isKindOf(scrollView, "NSScrollView") {
		st.stopScrollTimer()
		return
	}
	window := msg(scrollView, "window")
	locInWindow := objc.Send[objc.Point](objc.ID(uintptr(event)), objc.RegisterName("locationInWindow"))
	mouseLoc := objc.Send[objc.Point](objc.ID(uintptr(window)), objc.RegisterName("convertPointToScreen:"), locInWindow)
	windowFrame := objc.Send[objc.Rect](objc.ID(uintptr(window)), objc.RegisterName("frame"))

	const margin = 24.0
	const baseSpeed = 5.0
	st.scrollDelta = objc.Point{}

	minX := windowFrame.Origin.X
	maxX := windowFrame.Origin.X + windowFrame.Size.Width
	minY := windowFrame.Origin.Y
	maxY := windowFrame.Origin.Y + windowFrame.Size.Height

	if mouseLoc.X < minX+margin {
		st.scrollDelta.X = -baseSpeed
	} else if mouseLoc.X > maxX-margin {
		st.scrollDelta.X = baseSpeed
	}

	contentFrame := objc.Send[objc.Rect](objc.ID(uintptr(msg(window, "contentView"))), objc.RegisterName("frame"))
	titleBarHeight := windowFrame.Size.Height - contentFrame.Size.Height

	switch {
	case mouseLoc.Y >= maxY-titleBarHeight:
		st.scrollDelta.Y = 0
	case mouseLoc.Y < minY+margin:
		st.scrollDelta.Y = -baseSpeed
	case mouseLoc.Y > maxY-margin-titleBarHeight:
		st.scrollDelta.Y = baseSpeed
	}

	if st.scrollDelta.X != 0 || st.scrollDelta.Y != 0 {
		st.startScrollTimer()
	} else {
		st.stopScrollTimer()
	}
}

func (st *graphicWindow) startScrollTimer() {
	if st.scrollTimer == nil {
		st.scrollTimer = msgClass("NSTimer", "scheduledTimerWithTimeInterval:target:selector:userInfo:repeats:",
			float64(1.0/60.0), st.delegate, objc.RegisterName("scrollTick:"), unsafe.Pointer(nil), true)
	}
}

func (st *graphicWindow) stopScrollTimer() {
	if st.scrollTimer != nil {
		msgv(st.scrollTimer, "invalidate")
		st.scrollTimer = nil
	}
}

func (st *graphicWindow) scrollTick() {
	scrollView := msg(st.window, "contentView")
	if !isKindOf(scrollView, "NSScrollView") {
		st.stopScrollTimer()
		return
	}
	clipView := msg(scrollView, "contentView")
	clipBounds := objc.Send[objc.Rect](objc.ID(uintptr(clipView)), objc.RegisterName("bounds"))
	origin := clipBounds.Origin
	origin.X += st.scrollDelta.X
	origin.Y += st.scrollDelta.Y

	docView := msg(clipView, "documentView")
	docFrame := objc.Send[objc.Rect](objc.ID(uintptr(docView)), objc.RegisterName("frame"))
	origin.X = max(0, min(origin.X, docFrame.Size.Width-clipBounds.Size.Width))
	origin.Y = max(0, min(origin.Y, docFrame.Size.Height-clipBounds.Size.Height))
	msgv(clipView, "setBoundsOrigin:", origin)
}

// --- menu bar ---

func (st *graphicWindow) setupMenuBar() {
	menuBar := newObj("NSMenu", "init")
	menuBarItem := newObj("NSMenuItem", "init")
	msgv(menuBar, "addItem:", menuBarItem)
	msgv(st.app, "setMainMenu:", menuBar)

	msgv(menuBarItem, "setSubmenu:", st.setupApplicationMenu())

	windowMenuItem := menuItem("Window", "", "")
	msgv(menuBar, "addItem:", windowMenuItem)
	msgv(windowMenuItem, "setSubmenu:", st.setupWindowMenu())

	helpMenuItem := menuItem("Help", "", "")
	msgv(menuBar, "addItem:", helpMenuItem)
	msgv(helpMenuItem, "setSubmenu:", st.setupHelpMenu())
}

func (st *graphicWindow) setupApplicationMenu() unsafe.Pointer {
	appMenu := newObj("NSMenu", "init")
	appName := processName()

	aboutItem := menuItem("About "+appName, "openAboutWindow:", "")

	capturesItem := menuItem("Enable to send system hot keys to virtual machine", "toggleCapturesSystemKeys:", "")
	msgv(capturesItem, "setState:", st.capturesSystemKeysState())

	servicesItem := menuItem("Services", "", "")
	servicesMenu := newObj("NSMenu", "initWithTitle:", nsStr("Services"))
	msgv(servicesItem, "setSubmenu:", servicesMenu)
	msgv(st.app, "setServicesMenu:", servicesMenu)

	hideOthersItem := menuItem("Hide Others", "hideOtherApplications:", "h")
	msgv(hideOthersItem, "setKeyEquivalentModifierMask:", uint(nsEventModifierFlagOption|nsEventModifierFlagCommand))

	items := []unsafe.Pointer{
		aboutItem,
		separatorItem(),
		capturesItem,
		separatorItem(),
		servicesItem,
		separatorItem(),
		menuItem("Hide "+appName, "hide:", "h"),
		hideOthersItem,
		separatorItem(),
		menuItem("Quit "+appName, "terminate:", "q"),
	}
	for _, it := range items {
		msgv(appMenu, "addItem:", it)
	}
	return appMenu
}

func (st *graphicWindow) setupWindowMenu() unsafe.Pointer {
	windowMenu := newObj("NSMenu", "initWithTitle:", nsStr("Window"))
	items := []unsafe.Pointer{
		menuItem("Minimize", "performMiniaturize:", "m"),
		menuItem("Zoom", "performZoom:", ""),
		separatorItem(),
		menuItem("Bring All to Front", "arrangeInFront:", ""),
	}
	for _, it := range items {
		msgv(windowMenu, "addItem:", it)
	}
	msgv(st.app, "setWindowsMenu:", windowMenu)
	return windowMenu
}

func (st *graphicWindow) setupHelpMenu() unsafe.Pointer {
	helpMenu := newObj("NSMenu", "initWithTitle:", nsStr("Help"))
	msgv(helpMenu, "addItem:", menuItem("Report issue", "reportIssue:", ""))
	msgv(st.app, "setHelpMenu:", helpMenu)
	return helpMenu
}

func (st *graphicWindow) capturesSystemKeysState() int {
	if objc.Send[bool](objc.ID(uintptr(st.view)), objc.RegisterName("capturesSystemKeys")) {
		return nsControlStateValueOn
	}
	return nsControlStateValueOff
}

func (st *graphicWindow) toggleCapturesSystemKeys(sender unsafe.Pointer) {
	cur := objc.Send[bool](objc.ID(uintptr(st.view)), objc.RegisterName("capturesSystemKeys"))
	msgv(st.view, "setCapturesSystemKeys:", !cur)
	msgv(sender, "setState:", st.capturesSystemKeysState())
}

func (st *graphicWindow) reportIssue() {
	ws := msgClass("NSWorkspace", "sharedWorkspace")
	url := msgClass("NSURL", "URLWithString:", nsStr("https://github.com/Code-Hex/vz/issues/new"))
	msgv(ws, "openURL:", url)
}

func (st *graphicWindow) openAboutWindow() {
	panel := makeAboutPanel()
	msgv(panel, "makeKeyAndOrderFront:", unsafe.Pointer(nil))
}

// --- About panel ---

func makeAboutPanel() unsafe.Pointer {
	styleMask := uint(nsWindowStyleMaskTitled | nsWindowStyleMaskClosable)
	panel := newObj("NSPanel", "initWithContentRect:styleMask:backing:defer:",
		objc.Rect{}, styleMask, uint(nsBackingStoreBuffered), false)

	vc := newObj("NSViewController", "init")
	msgv(vc, "setView:", makeAboutContentView())
	msgv(panel, "setContentViewController:", vc)

	msgv(panel, "setTitleVisibility:", uint(nsWindowTitleHidden))
	msgv(panel, "setTitlebarAppearsTransparent:", true)
	msgv(panel, "setBecomesKeyOnlyIfNeeded:", false)
	msgv(panel, "center")
	return panel
}

func makeAboutContentView() unsafe.Pointer {
	view := newObj("NSView", "init")

	appIcon := msg(sharedApp(), "applicationIconImage")
	imageView := msgClass("NSImageView", "imageViewWithImage:", appIcon)

	appLabel := makeAboutLabel(nsStr(processName()))
	msgv(appLabel, "setFont:", msgClass("NSFont", "boldSystemFontOfSize:", float64(16)))

	subLabel := makePoweredByLabel()

	stackView := msgClass("NSStackView", "stackViewWithViews:", makeNSArray(imageView, appLabel, subLabel))
	msgv(stackView, "setOrientation:", int(nsUserInterfaceLayoutOrientationVertical))
	msgv(stackView, "setDistribution:", int(nsStackViewDistributionFillProportionally))
	msgv(stackView, "setSpacing:", float64(10))
	msgv(stackView, "setAlignment:", int(nsLayoutAttributeCenterX))
	msgv(stackView, "setContentCompressionResistancePriority:forOrientation:",
		float32(nsLayoutPriorityRequired), int(nsLayoutConstraintOrientationHorizontal))
	msgv(stackView, "setContentCompressionResistancePriority:forOrientation:",
		float32(nsLayoutPriorityRequired), int(nsLayoutConstraintOrientationVertical))

	msgv(view, "addSubview:", stackView)

	constraints := []unsafe.Pointer{
		constraintConstant(msg(imageView, "widthAnchor"), 80),
		constraintConstant(msg(imageView, "heightAnchor"), 80),
		constraintToAnchor(msg(stackView, "topAnchor"), msg(view, "topAnchor"), 4),
		constraintToAnchor(msg(stackView, "bottomAnchor"), msg(view, "bottomAnchor"), -16),
		constraintToAnchor(msg(stackView, "leadingAnchor"), msg(view, "leadingAnchor"), 32),
		constraintToAnchor(msg(stackView, "trailingAnchor"), msg(view, "trailingAnchor"), -32),
		constraintConstant(msg(stackView, "widthAnchor"), 300),
	}
	msgClass("NSLayoutConstraint", "activateConstraints:", makeNSArray(constraints...))
	return view
}

func makeAboutLabel(labelStr unsafe.Pointer) unsafe.Pointer {
	label := msgClass("NSTextField", "labelWithString:", labelStr)
	msgv(label, "setTextColor:", msgClass("NSColor", "labelColor"))
	msgv(label, "setEditable:", false)
	msgv(label, "setSelectable:", false)
	msgv(label, "setBezeled:", false)
	msgv(label, "setBordered:", false)
	msgv(label, "setBackgroundColor:", msgClass("NSColor", "clearColor"))
	msgv(label, "setAlignment:", int(nsTextAlignmentCenter))
	msgv(label, "setLineBreakMode:", uint(0)) // NSLineBreakByWordWrapping
	msgv(label, "setUsesSingleLineMode:", false)
	msgv(label, "setMaximumNumberOfLines:", int(20))
	return label
}

func makePoweredByLabel() unsafe.Pointer {
	poweredByAttr := newObj("NSMutableAttributedString", "initWithString:attributes:",
		nsStr("Powered by "),
		dictOf(nsForegroundColorAttrName, msgClass("NSColor", "labelColor")))

	repoURL := msgClass("NSURL", "URLWithString:", nsStr("https://github.com/Code-Hex/vz"))
	repository := makeHyperLink(nsStr("github.com/Code-Hex/vz"), repoURL)
	msgv(poweredByAttr, "appendAttributedString:", repository)

	length := objc.Send[uint64](objc.ID(uintptr(poweredByAttr)), objc.RegisterName("length"))
	msgv(poweredByAttr, "addAttribute:value:range:",
		nsFontAttrName, msgClass("NSFont", "systemFontOfSize:", float64(12)),
		objc.Range{Location: 0, Length: uint(length)})

	label := makeAboutLabel(nsStr(""))
	msgv(label, "setSelectable:", true)
	msgv(label, "setAllowsEditingTextAttributes:", true)
	msgv(label, "setAttributedStringValue:", poweredByAttr)
	return label
}

func makeHyperLink(inString, url unsafe.Pointer) unsafe.Pointer {
	attr := newObj("NSMutableAttributedString", "initWithString:", inString)
	length := objc.Send[uint64](objc.ID(uintptr(attr)), objc.RegisterName("length"))
	rng := objc.Range{Location: 0, Length: uint(length)}

	msgv(attr, "beginEditing")
	msgv(attr, "addAttribute:value:range:", nsLinkAttrName, msg(url, "absoluteString"), rng)
	msgv(attr, "addAttribute:value:range:", nsForegroundColorAttrName, msgClass("NSColor", "blueColor"), rng)
	msgv(attr, "addAttribute:value:range:", nsUnderlineStyleAttrName, numberInt(nsUnderlineStyleSingle), rng)
	msgv(attr, "endEditing")
	return attr
}

// --- small AppKit helpers ---

func isKindOf(obj unsafe.Pointer, class string) bool {
	return objc.Send[bool](objc.ID(uintptr(obj)), objc.RegisterName("isKindOfClass:"), objc.GetClass(class))
}

func sharedApp() unsafe.Pointer { return msgClass("NSApplication", "sharedApplication") }

func processName() string {
	return objc.GoString(msg(msg(msgClass("NSProcessInfo", "processInfo"), "processName"), "UTF8String"))
}

func separatorItem() unsafe.Pointer { return msgClass("NSMenuItem", "separatorItem") }

func menuItem(title, action, key string) unsafe.Pointer {
	var sel objc.SEL
	if action != "" {
		sel = objc.RegisterName(action)
	}
	return newObj("NSMenuItem", "initWithTitle:action:keyEquivalent:", nsStr(title), sel, nsStr(key))
}

func symbolImage(name string) unsafe.Pointer {
	return msgClass("NSImage", "imageWithSystemSymbolName:accessibilityDescription:", nsStr(name), unsafe.Pointer(nil))
}

func numberInt(n int) unsafe.Pointer {
	return msgClass("NSNumber", "numberWithInt:", int32(n))
}

func dictOf(key, value unsafe.Pointer) unsafe.Pointer {
	return msgClass("NSDictionary", "dictionaryWithObject:forKey:", value, key)
}

func constraintConstant(anchor unsafe.Pointer, c float64) unsafe.Pointer {
	return msg(anchor, "constraintEqualToConstant:", c)
}

func constraintToAnchor(a, b unsafe.Pointer, c float64) unsafe.Pointer {
	return msg(a, "constraintEqualToAnchor:constant:", b, c)
}
