package objc

import (
	"bytes"
	"testing"
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

// TestDefineClassReturnMarshaling is the return-valued IMP probe. It de-risks the one
// genuinely novel FFI mechanism the custom-Virtio save/restore delegate needs: a Go IMP
// registered through DefineClass that RETURNS a value (an NSData* and a BOOL) rather than
// void, marshaled back through purego's C->Go callback into the Objective-C result
// register.
//
// It registers a throwaway class with two return-valued IMPs, then invokes their
// selectors on a serial dispatch queue (the same execution context the framework uses for
// a device delegate) and asserts the returned NSData* bytes and BOOL round-trip. The empty
// and nil cases confirm AutoreleasedNSData yields a valid empty NSData (a non-nil object),
// never a nil pointer — the behavior the save-state delegate relies on to signal "saved,
// no state" instead of "save failed".
//
// This covers purego's marshaling of the return only; the framework's +0 ownership
// contract on the returned NSData is exercised by a running save/restore cycle (manual
// e2e), not here.
func TestDefineClassReturnMarshaling(t *testing.T) {
	// probeState holds the bytes the data IMP should return; recovered per-instance.
	type probeState struct{ data []byte }

	dataIMP := func(self ID, _ SEL) unsafe.Pointer {
		if v, ok := Associated(uintptr(self)).(*probeState); ok {
			return AutoreleasedNSData(v.data)
		}
		return AutoreleasedNSData(nil)
	}
	// echoIMP returns a BOOL derived from a framework-vended NSData* argument, mirroring
	// the shape of customVirtioDeviceShouldRestore:saveState: (a pointer arg -> BOOL).
	echoIMP := func(_ ID, _ SEL, arg unsafe.Pointer) bool {
		return len(NSDataToBytes(arg)) > 0
	}

	cls, err := DefineClass("VZGoReturnMarshalProbe", NSObjectClass(), []MethodDef{
		{Cmd: RegisterName("probeReturnData"), Fn: dataIMP},
		{Cmd: RegisterName("probeEchoNonEmpty:"), Fn: echoIMP},
	})
	if err != nil {
		t.Fatalf("DefineClass: %v", err)
	}

	obj := NewObject(cls)
	st := &probeState{}
	Associate(uintptr(obj), st)
	defer Disassociate(uintptr(obj))

	queue := DispatchQueueCreate("VZGoReturnMarshalProbe")
	defer ReleaseDispatch(queue)

	tests := map[string]struct {
		data         []byte
		wantNonEmpty bool
	}{
		"nil data":    {data: nil, wantNonEmpty: false},
		"empty data":  {data: []byte{}, wantNonEmpty: false},
		"small data":  {data: []byte{0x01, 0x02, 0x03}, wantNonEmpty: true},
		"larger data": {data: bytes.Repeat([]byte{0xAB}, 300), wantNonEmpty: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			st.data = tt.data

			var gotBytes []byte
			var gotBool bool
			// Invoke on the serial queue, exactly where a device delegate's methods run.
			DispatchSync(queue, func() {
				data := pobjc.Send[unsafe.Pointer](objcID(obj), RegisterName("probeReturnData"))
				gotBytes = NSDataToBytes(data)
				gotBool = pobjc.Send[bool](objcID(obj), RegisterName("probeEchoNonEmpty:"), data)
			})

			// The NSData* return always round-trips to a valid (never nil) object, even
			// for nil/empty input: NSDataToBytes returns a non-nil slice for a real NSData.
			if gotBytes == nil {
				t.Fatalf("probeReturnData: NSDataToBytes returned nil; AutoreleasedNSData must yield a valid empty NSData, not a nil pointer")
			}
			want := tt.data
			if want == nil {
				want = []byte{}
			}
			if !bytes.Equal(gotBytes, want) {
				t.Errorf("NSData* return round-trip = %v, want %v", gotBytes, want)
			}
			// The BOOL return round-trips both true and false across the table.
			if gotBool != tt.wantNonEmpty {
				t.Errorf("BOOL return = %v, want %v", gotBool, tt.wantNonEmpty)
			}
		})
	}
}
