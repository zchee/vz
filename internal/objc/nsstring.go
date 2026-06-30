package objc

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

// libc allocator, used for C strings and out-parameter slots that Objective-C
// methods write into. Allocating in C memory (rather than passing a Go pointer)
// keeps these buffers off the Go heap so the GC never has to pin them.
var (
	libcMalloc func(size uint64) unsafe.Pointer
	libcFree   func(p unsafe.Pointer)
)

func init() {
	purego.RegisterLibFunc(&libcMalloc, purego.RTLD_DEFAULT, "malloc")
	purego.RegisterLibFunc(&libcFree, purego.RTLD_DEFAULT, "free")
}

// CStringMalloc returns a NUL-terminated C copy of s allocated with libc malloc.
// Release it with Free.
func CStringMalloc(s string) unsafe.Pointer {
	n := len(s)
	p := libcMalloc(uint64(n) + 1)
	if p == nil {
		panic("objc: malloc failed")
	}
	dst := unsafe.Slice((*byte)(p), n+1)
	copy(dst, s)
	dst[n] = 0
	return p
}

// GoString copies a NUL-terminated C string into a Go string. It returns the
// empty string for a nil pointer.
func GoString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// Free releases memory returned by CStringMalloc or NewErrorSlot.
func Free(p unsafe.Pointer) {
	if p != nil {
		libcFree(p)
	}
}
