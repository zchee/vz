package objc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleRegistry(t *testing.T) {
	h := NewHandle("hello")
	if h == 0 {
		t.Fatal("NewHandle returned the reserved zero handle")
	}
	if got, ok := h.Value().(string); !ok || got != "hello" {
		t.Fatalf("Value() = %v, want %q", h.Value(), "hello")
	}
	h.Delete()
	if h.Value() != nil {
		t.Fatalf("Value() after Delete = %v, want nil", h.Value())
	}
	// The zero handle always resolves to nil.
	if Handle(0).Value() != nil {
		t.Fatal("zero handle must resolve to nil")
	}
}

func TestHandleRegistryConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h := NewHandle(n)
			if got, ok := h.Value().(int); !ok || got != n {
				t.Errorf("concurrent Value() = %v, want %d", h.Value(), n)
			}
			h.Delete()
		}(i)
	}
	wg.Wait()
}

func TestDispatchSync(t *testing.T) {
	q := DispatchQueueCreate("vz.test.sync")
	defer ReleaseDispatch(q)

	var ran atomic.Bool
	DispatchSync(q, func() { ran.Store(true) })
	if !ran.Load() {
		t.Fatal("DispatchSync did not run the function synchronously")
	}
}

// TestDispatchSyncReentrancy guards against the classic dispatch_sync deadlock:
// issuing a synchronous dispatch to the queue you are already running on. The
// whole test is bounded by a timeout so a regression fails fast instead of
// hanging the suite.
func TestDispatchSyncReentrancy(t *testing.T) {
	q := DispatchQueueCreate("vz.test.reentrant")
	defer ReleaseDispatch(q)

	done := make(chan struct{})
	var inner atomic.Bool
	go func() {
		DispatchSync(q, func() {
			// Already on q: this nested DispatchSync must run inline.
			DispatchSync(q, func() { inner.Store(true) })
		})
		close(done)
	}()

	select {
	case <-done:
		if !inner.Load() {
			t.Fatal("nested DispatchSync did not run")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchSync deadlocked on same-queue reentrancy")
	}
}

func TestDispatchAsync(t *testing.T) {
	q := DispatchQueueCreate("vz.test.async")
	defer ReleaseDispatch(q)

	done := make(chan struct{})
	DispatchAsync(q, func() { close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchAsync never ran the function")
	}
}
