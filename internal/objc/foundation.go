package objc

import (
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
