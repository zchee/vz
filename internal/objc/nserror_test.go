package objc

import (
	"testing"
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

func TestCStringRoundTrip(t *testing.T) {
	tests := map[string]struct{ in string }{
		"empty":     {in: ""},
		"ascii":     {in: "hello"},
		"path":      {in: "/var/db/with spaces"},
		"multibyte": {in: "日本語パス"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := CStringMalloc(tt.in)
			defer Free(p)
			if got := GoString(p); got != tt.in {
				t.Errorf("GoString(CStringMalloc(%q)) = %q", tt.in, got)
			}
		})
	}
	if GoString(nil) != "" {
		t.Error("GoString(nil) must be empty")
	}
}

// makeNSError builds a real NSError so the readers can be exercised without a
// failing framework call.
func makeNSError(domain string, code int) unsafe.Pointer {
	dom := pobjc.ID(classNSString).Send(selStringWithUTF8String, append([]byte(domain), 0))
	return pobjc.Send[unsafe.Pointer](
		pobjc.ID(pobjc.GetClass("NSError")),
		pobjc.RegisterName("errorWithDomain:code:userInfo:"),
		dom, int64(code), pobjc.ID(0),
	)
}

func TestErrorReaders(t *testing.T) {
	err := makeNSError("VZTestDomain", 42)
	if err == nil {
		t.Fatal("failed to create NSError")
	}
	if got := ErrorCode(err); got != 42 {
		t.Errorf("ErrorCode = %d, want 42", got)
	}
	if got := ErrorDomain(err); got != "VZTestDomain" {
		t.Errorf("ErrorDomain = %q, want VZTestDomain", got)
	}
	if ErrorLocalizedDescription(err) == "" {
		t.Error("ErrorLocalizedDescription must be non-empty for a coded error")
	}
}

func TestErrorSlot(t *testing.T) {
	slot := NewErrorSlot()
	defer Free(slot)

	if HasError(slot) {
		t.Fatal("a fresh slot must report no error")
	}
	if ErrorCode(ErrorFromSlot(slot)) != 0 {
		t.Error("code of a nil error must be 0")
	}

	// Simulate Objective-C writing *error = someError.
	err := makeNSError("SlotDomain", 7)
	*(*unsafe.Pointer)(slot) = err

	if !HasError(slot) {
		t.Fatal("slot must report an error after a write")
	}
	if ErrorFromSlot(slot) != err {
		t.Error("ErrorFromSlot returned a different pointer")
	}
	if got := ErrorCode(ErrorFromSlot(slot)); got != 7 {
		t.Errorf("slot error code = %d, want 7", got)
	}
}
