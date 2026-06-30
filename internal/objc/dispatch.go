package objc

import (
	"unsafe"

	"github.com/ebitengine/purego"
	pobjc "github.com/ebitengine/purego/objc"
)

// libdispatch primitives. Queue handles cross the boundary as uintptr so that
// the data-symbol main queue and pointer-typed created queues share one path
// without any vet-flagged uintptr-to-pointer conversions.
var (
	dispatchQueueCreateFn func(label []byte, attr uintptr) unsafe.Pointer
	dispatchSyncFn        func(queue, block uintptr)
	dispatchAsyncFn       func(queue, block uintptr)
	dispatchSetSpecificFn func(queue, key, context, destructor uintptr)
	dispatchGetSpecificFn func(key uintptr) uintptr

	// mainQueue is the address of the _dispatch_main_q data symbol, which is
	// exactly what dispatch_get_main_queue() returns.
	mainQueue uintptr
)

// reentrancyKeyStorage gives a stable, unique address used as the
// dispatch_queue_set_specific key. Its address never changes (package-level
// storage is not moved by the GC).
var reentrancyKeyStorage byte

func reentrancyKey() uintptr {
	return uintptr(unsafe.Pointer(&reentrancyKeyStorage))
}

func init() {
	purego.RegisterLibFunc(&dispatchQueueCreateFn, purego.RTLD_DEFAULT, "dispatch_queue_create")
	purego.RegisterLibFunc(&dispatchSyncFn, purego.RTLD_DEFAULT, "dispatch_sync")
	purego.RegisterLibFunc(&dispatchAsyncFn, purego.RTLD_DEFAULT, "dispatch_async")
	purego.RegisterLibFunc(&dispatchSetSpecificFn, purego.RTLD_DEFAULT, "dispatch_queue_set_specific")
	purego.RegisterLibFunc(&dispatchGetSpecificFn, purego.RTLD_DEFAULT, "dispatch_get_specific")
	if sym, err := purego.Dlsym(purego.RTLD_DEFAULT, "_dispatch_main_q"); err == nil {
		mainQueue = sym
	}
}

// DispatchQueueCreate creates a serial dispatch queue with the given label. The
// queue is tagged so DispatchSync can detect same-queue reentrancy. Release it
// with ReleaseDispatch when no longer needed.
func DispatchQueueCreate(label string) unsafe.Pointer {
	q := dispatchQueueCreateFn(append([]byte(label), 0), 0)
	// Tag the queue with its own address so a DispatchSync issued from a block
	// already running on this queue can be detected.
	dispatchSetSpecificFn(uintptr(q), reentrancyKey(), uintptr(q), 0)
	return q
}

// DispatchSync runs fn on queue and blocks until it returns. If the caller is
// already executing on queue, fn is run inline instead of via dispatch_sync,
// which would otherwise deadlock.
func DispatchSync(queue unsafe.Pointer, fn func()) {
	if dispatchGetSpecificFn(reentrancyKey()) == uintptr(queue) {
		fn()
		return
	}
	block := pobjc.NewBlock(func(pobjc.Block) { fn() })
	defer block.Release()
	dispatchSyncFn(uintptr(queue), uintptr(block))
}

// DispatchAsync schedules fn to run on queue and returns immediately.
func DispatchAsync(queue unsafe.Pointer, fn func()) {
	block := pobjc.NewBlock(func(pobjc.Block) { fn() })
	// dispatch_async takes its own copy (Block_copy) of the block, so the local
	// reference can be released as soon as it is enqueued.
	dispatchAsyncFn(uintptr(queue), uintptr(block))
	block.Release()
}

// DispatchAsyncMain schedules fn to run on the main dispatch queue. It is used
// for UI work that must run on the main thread.
func DispatchAsyncMain(fn func()) {
	block := pobjc.NewBlock(func(pobjc.Block) { fn() })
	dispatchAsyncFn(mainQueue, uintptr(block))
	block.Release()
}
