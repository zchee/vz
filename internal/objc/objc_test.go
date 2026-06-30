package objc

import (
	"sync"
	"testing"
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

// makeNSString returns an autoreleased NSString* as a raw pointer.
func makeNSString(s string) unsafe.Pointer {
	return pobjc.Send[unsafe.Pointer](pobjc.ID(classNSString), selStringWithUTF8String, append([]byte(s), 0))
}

// cStringLen reports the length of a NUL-terminated C string.
func cStringLen(p unsafe.Pointer) int {
	if p == nil {
		return 0
	}
	for i := 0; ; i++ {
		if *(*byte)(unsafe.Add(p, i)) == 0 {
			return i
		}
	}
}

func TestConvertToNSMutableArrayRoundTrip(t *testing.T) {
	tests := map[string]struct {
		values []string
	}{
		"success: three elements": {values: []string{"alpha", "beta", "gamma"}},
		"success: empty":          {values: nil},
		"success: single":         {values: []string{"only"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			objs := make([]NSObject, len(tt.values))
			want := make([]unsafe.Pointer, len(tt.values))
			for i, v := range tt.values {
				p := makeNSString(v)
				want[i] = p
				objs[i] = NewPointer(p)
			}

			arr := ConvertToNSMutableArray(objs)
			got := NewNSArray(Ptr(arr)).ToPointerSlice()

			if len(got) != len(tt.values) {
				t.Fatalf("ToPointerSlice len = %d, want %d", len(got), len(tt.values))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("element %d = %p, want %p", i, got[i], want[i])
				}
			}
		})
	}
}

func TestConvertToNSMutableDictionary(t *testing.T) {
	d := map[string]NSObject{
		"one":   NewPointer(makeNSString("1")),
		"two":   NewPointer(makeNSString("2")),
		"three": NewPointer(makeNSString("3")),
	}
	dict := ConvertToNSMutableDictionary(d)
	count := pobjc.Send[uint64](objcID(Ptr(dict)), selCount)
	if int(count) != len(d) {
		t.Fatalf("dictionary count = %d, want %d", count, len(d))
	}
}

func TestGetUUID(t *testing.T) {
	p := GetUUID()
	if p == nil {
		t.Fatal("GetUUID returned nil")
	}
	// A UUID string is canonically 36 characters: 8-4-4-4-12 plus 4 hyphens.
	if got := cStringLen(p); got != 36 {
		t.Errorf("UUID string length = %d, want 36", got)
	}
	// Two successive calls must differ.
	if cStringLen(GetUUID()) != 36 {
		t.Error("second GetUUID not a UUID-length string")
	}
}

func TestRetainRelease(t *testing.T) {
	// Retain then release must be balanced and must not crash.
	p := NewPointer(makeNSString("retainable"))
	Retain(p)
	Release(p)
}

func TestConvertToNSMutableArrayConcurrent(t *testing.T) {
	// Exercises concurrent message sends for the race detector.
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			objs := []NSObject{
				NewPointer(makeNSString("x")),
				NewPointer(makeNSString("y")),
			}
			if got := NewNSArray(Ptr(ConvertToNSMutableArray(objs))).ToPointerSlice(); len(got) != 2 {
				t.Errorf("concurrent ToPointerSlice len = %d, want 2", len(got))
			}
		}()
	}
	wg.Wait()
}
