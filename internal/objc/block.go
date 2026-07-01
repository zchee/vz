package objc

import (
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

// Block is an Objective-C block handle. Build one from a Go func with the
// Block* constructors, pass it to a method expecting a completionHandler:, and
// the captured Go closure stays alive until the framework releases the block.
type Block = pobjc.Block

// BlockVoid builds a void(^)(void) block.
func BlockVoid(fn func()) Block {
	return pobjc.NewBlock(func(Block) { fn() })
}

// BlockError builds a void(^)(NSError *) completion block. The handler receives
// the NSError as a raw pointer, nil when the operation succeeded.
func BlockError(fn func(err unsafe.Pointer)) Block {
	return pobjc.NewBlock(func(_ Block, err unsafe.Pointer) { fn(err) })
}

// BlockObjectError builds a void(^)(id, NSError *) completion block.
func BlockObjectError(fn func(obj, err unsafe.Pointer)) Block {
	return pobjc.NewBlock(func(_ Block, obj, err unsafe.Pointer) { fn(obj, err) })
}

// BlockBoolError builds a void(^)(BOOL, NSError *) completion block.
func BlockBoolError(fn func(ok bool, err unsafe.Pointer)) Block {
	return pobjc.NewBlock(func(_ Block, ok bool, err unsafe.Pointer) { fn(ok, err) })
}

// BlockDouble builds a void(^)(double) block (e.g. installation progress).
func BlockDouble(fn func(value float64)) Block {
	return pobjc.NewBlock(func(_ Block, value float64) { fn(value) })
}

// BlockObject builds a void(^)(id) block for callbacks that receive a single
// object argument, such as the animation block of
// +[NSAnimationContext runAnimationGroup:completionHandler:].
func BlockObject(fn func(obj unsafe.Pointer)) Block {
	return pobjc.NewBlock(func(_ Block, obj unsafe.Pointer) { fn(obj) })
}

// BlockEventMonitor builds an NSEvent *(^)(NSEvent *) block for
// -[NSEvent addLocalMonitorForEventsMatchingMask:handler:]. Returning the event
// passes it along the responder chain; returning nil swallows it.
func BlockEventMonitor(fn func(event unsafe.Pointer) unsafe.Pointer) Block {
	return pobjc.NewBlock(func(_ Block, event unsafe.Pointer) unsafe.Pointer {
		return fn(event)
	})
}
