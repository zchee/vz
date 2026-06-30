package objc

import (
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

// Re-exported purego/objc primitives so the parent package depends only on this
// internal package for Objective-C messaging.
type (
	ID    = pobjc.ID
	SEL   = pobjc.SEL
	Class = pobjc.Class
)

// GetClass returns the Objective-C class registered under name.
func GetClass(name string) Class { return pobjc.GetClass(name) }

// RegisterName returns the selector for the method named name.
func RegisterName(name string) SEL { return pobjc.RegisterName(name) }

// Send sends sel to id and returns the result typed as T. See purego objc.Send
// for the supported return kinds (integers, bool, float, unsafe.Pointer,
// arm64 struct returns).
func Send[T any](id ID, sel SEL, args ...any) T { return pobjc.Send[T](id, sel, args...) }

// New allocates an instance of className and sends initSel to it, returning the
// new object as a raw pointer suitable for NewPointer. The object has a +1
// retain count, so pair it with a release finalizer.
func New(className, initSel string, args ...any) unsafe.Pointer {
	obj := pobjc.ID(GetClass(className)).Send(selAlloc)
	return pobjc.Send[unsafe.Pointer](obj, RegisterName(initSel), args...)
}

// SendVoid sends sel (whose return value is unused) to the object at target.
func SendVoid(target unsafe.Pointer, sel string, args ...any) {
	objcID(target).Send(RegisterName(sel), args...)
}

// SendPtr sends sel to the object at target and returns an object pointer.
func SendPtr(target unsafe.Pointer, sel string, args ...any) unsafe.Pointer {
	return pobjc.Send[unsafe.Pointer](objcID(target), RegisterName(sel), args...)
}
