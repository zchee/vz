package objc

import (
	"unsafe"

	"github.com/ebitengine/purego"
	pobjc "github.com/ebitengine/purego/objc"
)

// This file provides the Cocoa/AppKit-oriented FFI helpers used by the
// graphical-application entrypoint. The frameworks are loaded lazily (via
// LoadFramework) rather than at package init, so headless virtual machines that
// never open a window do not pull in AppKit or touch the window server.

// CGFloat is the Core Graphics floating-point type (64-bit on arm64 and amd64).
type CGFloat = float64

// Point mirrors CGPoint / NSPoint.
type Point struct {
	X CGFloat
	Y CGFloat
}

// Size mirrors CGSize / NSSize.
type Size struct {
	Width  CGFloat
	Height CGFloat
}

// Rect mirrors CGRect / NSRect. It is a homogeneous aggregate of four CGFloats,
// so purego returns it in the SIMD registers and passes it by value as an
// argument, matching the arm64 procedure call standard.
type Rect struct {
	Origin Point
	Size   Size
}

// Range mirrors NSRange (two NSUInteger fields).
type Range struct {
	Location uint
	Length   uint
}

// LoadFramework dlopens a framework by path so its classes and extern data
// symbols resolve through the Objective-C runtime. It is safe to call more than
// once; subsequent calls return the existing handle.
func LoadFramework(path string) error {
	_, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	return err
}

// ExternPointer returns the pointer value held in an extern data symbol, such as
// an `extern NSString * const NSFooName`. It returns nil when the symbol cannot
// be resolved. The framework that defines the symbol must already be loaded.
func ExternPointer(name string) unsafe.Pointer {
	sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name)
	if err != nil || sym == 0 {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(sym))
}

// SendClass sends sel to the named class object and returns an object pointer.
// It is the class-method counterpart of SendPtr.
func SendClass(className, sel string, args ...any) unsafe.Pointer {
	return pobjc.Send[unsafe.Pointer](pobjc.ID(GetClass(className)), RegisterName(sel), args...)
}

// libobjc autorelease-pool primitives. A GUI event loop needs a top-level
// autorelease pool so autoreleased AppKit objects created while it runs are
// drained; these bind the runtime's pool push/pop entry points.
var (
	objcAutoreleasePoolPush func() unsafe.Pointer
	objcAutoreleasePoolPop  func(unsafe.Pointer)
)

func init() {
	purego.RegisterLibFunc(&objcAutoreleasePoolPush, purego.RTLD_DEFAULT, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&objcAutoreleasePoolPop, purego.RTLD_DEFAULT, "objc_autoreleasePoolPop")
}

// AutoreleasePoolPush pushes a new autorelease pool and returns its token.
func AutoreleasePoolPush() unsafe.Pointer { return objcAutoreleasePoolPush() }

// AutoreleasePoolPop drains the autorelease pool identified by token.
func AutoreleasePoolPop(token unsafe.Pointer) { objcAutoreleasePoolPop(token) }
