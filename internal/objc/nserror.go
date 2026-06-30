package objc

import (
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

var (
	selErrorCode            = pobjc.RegisterName("code")
	selErrorDomain          = pobjc.RegisterName("domain")
	selLocalizedDescription = pobjc.RegisterName("localizedDescription")
	selUserInfo             = pobjc.RegisterName("userInfo")
	selDescription          = pobjc.RegisterName("description")
)

// NewErrorSlot allocates a zeroed, pointer-sized slot in C memory for an
// Objective-C "NSError **error" out-parameter. Pass it to a method that takes
// an (NSError **) and read the result with HasError / ErrorFromSlot. Release it
// with Free. The slot lives in C memory so the GC never needs to pin it across
// the call.
func NewErrorSlot() unsafe.Pointer {
	p := libcMalloc(uint64(unsafe.Sizeof(uintptr(0))))
	if p == nil {
		panic("objc: malloc failed")
	}
	*(*unsafe.Pointer)(p) = nil
	return p
}

// ErrorFromSlot returns the NSError object written into the slot, or nil if the
// method reported no error.
func ErrorFromSlot(slot unsafe.Pointer) unsafe.Pointer {
	if slot == nil {
		return nil
	}
	return *(*unsafe.Pointer)(slot)
}

// HasError reports whether the slot holds an NSError.
func HasError(slot unsafe.Pointer) bool {
	return ErrorFromSlot(slot) != nil
}

// ErrorCode returns the NSError code.
func ErrorCode(err unsafe.Pointer) int {
	if err == nil {
		return 0
	}
	return int(pobjc.Send[int64](objcID(err), selErrorCode))
}

// ErrorDomain returns the NSError domain string.
func ErrorDomain(err unsafe.Pointer) string {
	return nsStringToGo(objcID(err).Send(selErrorDomain))
}

// ErrorLocalizedDescription returns the NSError localizedDescription string.
func ErrorLocalizedDescription(err unsafe.Pointer) string {
	return nsStringToGo(objcID(err).Send(selLocalizedDescription))
}

// ErrorUserInfo returns the NSError userInfo dictionary rendered as a string.
func ErrorUserInfo(err unsafe.Pointer) string {
	if err == nil {
		return ""
	}
	info := objcID(err).Send(selUserInfo)
	if info == 0 {
		return ""
	}
	return nsStringToGo(info.Send(selDescription))
}

// nsStringToGo converts an NSString id to a Go string via -UTF8String.
func nsStringToGo(ns pobjc.ID) string {
	if ns == 0 {
		return ""
	}
	return pobjc.Send[string](ns, selUTF8String)
}
