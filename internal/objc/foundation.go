package objc

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	pobjc "github.com/ebitengine/purego/objc"
)

const virtualizationFrameworkPath = "/System/Library/Frameworks/Virtualization.framework/Virtualization"

var (
	classNSURL         pobjc.Class
	selFileURLWithPath pobjc.SEL
)

func init() {
	// Load Virtualization so its VZ* classes resolve under CGO_ENABLED=0. When
	// the parent package is built with cgo the framework is already linked, so
	// this just returns the existing handle.
	_, _ = purego.Dlopen(virtualizationFrameworkPath, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	classNSURL = pobjc.GetClass("NSURL")
	selFileURLWithPath = pobjc.RegisterName("fileURLWithPath:")
}

// NSString returns an autoreleased NSString for s as a raw object pointer. The
// string is suitable for immediate use as a message argument.
func NSString(s string) unsafe.Pointer {
	return pobjc.Send[unsafe.Pointer](pobjc.ID(classNSString), selStringWithUTF8String, append([]byte(s), 0))
}

// FileURL returns an autoreleased file NSURL for the filesystem path as a raw
// object pointer.
func FileURL(path string) unsafe.Pointer {
	ns := pobjc.ID(classNSString).Send(selStringWithUTF8String, append([]byte(path), 0))
	return pobjc.Send[unsafe.Pointer](pobjc.ID(classNSURL), selFileURLWithPath, ns)
}

// NSData returns a newly allocated NSData (+1 retain count) copying b. NSData
// copies the bytes during initialization, so the Go backing array only needs to
// stay alive across this call. Release the result with a raw "release" message
// once the consuming object has taken its own reference.
func NSData(b []byte) unsafe.Pointer {
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	data := pobjc.Send[unsafe.Pointer](
		pobjc.ID(classNSData).Send(selAlloc),
		selInitWithBytesLength, p, uint64(len(b)),
	)
	runtime.KeepAlive(b)
	return data
}

// AutoreleasedNSData returns an NSData copying b, added to the current autorelease
// pool so its net retain count is +0, as a raw object pointer. Use it where a method
// returns an NSData * whose ownership it does not hand to the caller — for example a
// - (NSData *) delegate method whose result the framework copies and does not release.
//
// b may be nil or empty; the result is a valid empty NSData ([NSData data]-equivalent),
// never a nil pointer. Unlike NSData (which returns a +1 object the caller must
// release), the caller must NOT release this result. Do not wrap the call site in an
// AutoreleasePoolPush/AutoreleasePoolPop that drains before the consumer copies the
// bytes: that would free the object while the consumer still holds it. When used as an
// Objective-C method's return value the pool that eventually balances the autorelease is
// the caller's own (for a delegate invoked on a dispatch queue, the queue block's GCD
// autorelease pool), which drains only after the framework has taken its copy.
func AutoreleasedNSData(b []byte) unsafe.Pointer {
	data := NSData(b)
	objcID(data).Send(selAutorelease)
	return data
}

// NSDataToBytes copies the contents of an NSData object into a new Go byte
// slice. It returns nil for a nil data pointer. The copy makes the result
// independent of the NSData's lifetime.
func NSDataToBytes(data unsafe.Pointer) []byte {
	if data == nil {
		return nil
	}
	id := objcID(data)
	length := int(pobjc.Send[uint64](id, selLength))
	if length == 0 {
		return []byte{}
	}
	bytes := pobjc.Send[unsafe.Pointer](id, selBytes)
	out := make([]byte, length)
	copy(out, unsafe.Slice((*byte)(bytes), length))
	return out
}
