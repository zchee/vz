package objc

import (
	"sync/atomic"
	"testing"
	"unsafe"

	pobjc "github.com/ebitengine/purego/objc"
)

func TestBlockShapesCreate(t *testing.T) {
	// Creating each block validates its Objective-C type encoding; NewBlock
	// panics on an unsupported one.
	blocks := []Block{
		BlockVoid(func() {}),
		BlockError(func(unsafe.Pointer) {}),
		BlockObjectError(func(unsafe.Pointer, unsafe.Pointer) {}),
		BlockBoolError(func(bool, unsafe.Pointer) {}),
		BlockDouble(func(float64) {}),
	}
	for _, b := range blocks {
		b.Release()
	}
}

func TestBlockForeignInvoke(t *testing.T) {
	// NSNotificationCenter retains a void(^)(id) block and invokes it with the
	// posted NSNotification, exercising the C->Go block ABI with a pointer arg.
	center := pobjc.ID(GetClass("NSNotificationCenter")).Send(RegisterName("defaultCenter"))
	name := pobjc.ID(classNSString).Send(selStringWithUTF8String, append([]byte("Phase4Ping"), 0))

	var got unsafe.Pointer
	fired := 0
	blk := BlockError(func(note unsafe.Pointer) { fired++; got = note })
	defer blk.Release()

	token := center.Send(RegisterName("addObserverForName:object:queue:usingBlock:"),
		name, ID(0), ID(0), blk)
	defer center.Send(RegisterName("removeObserver:"), token)

	center.Send(RegisterName("postNotificationName:object:"), name, ID(0))
	if fired != 1 || got == nil {
		t.Fatalf("block foreign invoke: fired=%d got=%v", fired, got)
	}
}

func TestDefineClassKVOObserver(t *testing.T) {
	var fired int64
	var fraction float64
	imp := func(self ID, _ SEL, _, object, _ ID, _ uintptr) {
		atomic.AddInt64(&fired, 1)
		if v, ok := Associated(uintptr(self)).(*int); ok {
			*v = 42
		}
		fraction = Send[float64](object, RegisterName("fractionCompleted"))
	}
	cls, err := DefineClass("Phase4KVOObserver", NSObjectClass(), []MethodDef{
		{Cmd: RegisterName("observeValueForKeyPath:ofObject:change:context:"), Fn: imp},
	})
	if err != nil {
		t.Fatal(err)
	}

	observer := NewObject(cls)
	state := new(int)
	Associate(uintptr(observer), state)
	defer Disassociate(uintptr(observer))

	keyPath := func() ID {
		return pobjc.ID(classNSString).Send(selStringWithUTF8String, append([]byte("fractionCompleted"), 0))
	}
	prog := pobjc.ID(GetClass("NSProgress")).Send(RegisterName("progressWithTotalUnitCount:"), int64(100))
	prog.Send(RegisterName("addObserver:forKeyPath:options:context:"),
		observer, keyPath(), uint64(1) /*NSKeyValueObservingOptionNew*/, uintptr(0))
	prog.Send(RegisterName("setCompletedUnitCount:"), int64(50))
	prog.Send(RegisterName("removeObserver:forKeyPath:"), observer, keyPath())

	if atomic.LoadInt64(&fired) == 0 {
		t.Fatal("KVO IMP did not fire")
	}
	if *state != 42 {
		t.Errorf("per-instance associated state not recovered: got %d", *state)
	}
	if fraction < 0.49 || fraction > 0.51 {
		t.Errorf("fraction=%.3f, want ~0.5", fraction)
	}
}
