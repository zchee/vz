package objc

import (
	"sync"
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

type (
	// MethodDef pairs a selector with the Go func implementing it. The func must
	// take (ID, SEL) as its first two arguments.
	MethodDef = pobjc.MethodDef
	// IMP is an Objective-C method implementation pointer.
	IMP = pobjc.IMP
)

// DefineClass registers a new Objective-C class named name, subclassing super,
// with the given instance methods. It is the cgo-free replacement for the
// hand-written delegate/observer classes in the old Objective-C bridge.
func DefineClass(name string, super Class, methods []MethodDef) (Class, error) {
	return pobjc.RegisterClass(name, super, nil, nil, methods)
}

// NSObjectClass returns the NSObject class, the usual superclass for delegates.
func NSObjectClass() Class { return pobjc.GetClass("NSObject") }

// NewObject allocates and initializes an instance of cls.
func NewObject(cls Class) unsafe.Pointer {
	return pobjc.Send[unsafe.Pointer](pobjc.ID(cls).Send(selAlloc), selInit)
}

// Associated-object store. A class registered with DefineClass shares one IMP
// across all its instances, so per-instance Go state (the equivalent of the old
// cgoHandle ivar) is kept here, keyed by the instance address. An IMP recovers
// its state with Associated(uintptr(self)); callers must Disassociate when the
// instance is torn down.
var associated sync.Map // map[uintptr]any

// Associate stores per-instance Go state for the object at self (a uintptr-form
// object pointer).
func Associate(self uintptr, value any) { associated.Store(self, value) }

// Associated returns the Go state previously stored for self, or nil.
func Associated(self uintptr) any {
	v, _ := associated.Load(self)
	return v
}

// Disassociate removes the Go state stored for self.
func Disassociate(self uintptr) { associated.Delete(self) }
