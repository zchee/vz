package objc

import (
	"sync"
	"sync/atomic"
)

// Handle is an opaque integer reference to a Go value. It is a cgo-free
// replacement for runtime/cgo.Handle and is used the same way: register a Go
// value, store the returned Handle (a uintptr) in an Objective-C object's
// instance variable, look the value up when a delegate method or block fires,
// and delete it when the owning object is torn down.
//
// Because a Handle is a plain integer, it can cross the Objective-C boundary
// safely: the Go garbage collector never sees it as a pointer, so the
// underlying value cannot be collected while only an Objective-C object refers
// to it.
type Handle uintptr

var (
	handleMu      sync.RWMutex
	handleValues  = make(map[Handle]any)
	handleCounter atomic.Uint64
)

// NewHandle registers v and returns a Handle for it. The zero Handle is
// reserved to represent "no value", so handles are numbered from one.
func NewHandle(v any) Handle {
	h := Handle(handleCounter.Add(1))
	handleMu.Lock()
	handleValues[h] = v
	handleMu.Unlock()
	return h
}

// Value returns the value associated with the handle, or nil if the handle is
// the zero value or has been deleted.
func (h Handle) Value() any {
	handleMu.RLock()
	v := handleValues[h]
	handleMu.RUnlock()
	return v
}

// Delete removes the handle's association. It is safe to call with an unknown
// or already-deleted handle; subsequent Value calls return nil.
func (h Handle) Delete() {
	handleMu.Lock()
	delete(handleValues, h)
	handleMu.Unlock()
}
