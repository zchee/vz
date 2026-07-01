package objc

import "testing"

const cocoaPath = "/System/Library/Frameworks/Cocoa.framework/Cocoa"

// TestStructValueRoundTrip verifies that the geometry structs cross the
// Objective-C boundary correctly in both directions through objc.Send: as
// by-value arguments and as return values. NSValue boxes each struct and hands
// it straight back, so a mismatch would indicate a struct-ABI defect (the exact
// path the graphical application relies on for NSRect/NSSize/NSPoint).
func TestStructValueRoundTrip(t *testing.T) {
	// NSRange is a 16-byte integer aggregate; it is available from Foundation
	// alone and needs no AppKit.
	t.Run("NSRange", func(t *testing.T) {
		want := Range{Location: 3, Length: 7}
		box := SendClass("NSValue", "valueWithRange:", want)
		got := Send[Range](objcID(box), RegisterName("rangeValue"))
		if got != want {
			t.Fatalf("NSRange round-trip: got %+v, want %+v", got, want)
		}
	})

	// NSPoint/NSSize/NSRect are homogeneous float64 aggregates (HFAs), returned
	// and passed in the SIMD registers on arm64. The boxing accessors live in
	// AppKit's NSValue geometry category, so load Cocoa first.
	if err := LoadFramework(cocoaPath); err != nil {
		t.Skipf("Cocoa unavailable: %v", err)
	}

	tests := map[string]struct {
		check func(t *testing.T)
	}{
		"NSPoint": {check: func(t *testing.T) {
			want := Point{X: 1.5, Y: -2.25}
			box := SendClass("NSValue", "valueWithPoint:", want)
			if got := Send[Point](objcID(box), RegisterName("pointValue")); got != want {
				t.Fatalf("NSPoint round-trip: got %+v, want %+v", got, want)
			}
		}},
		"NSSize": {check: func(t *testing.T) {
			want := Size{Width: 640, Height: 480}
			box := SendClass("NSValue", "valueWithSize:", want)
			if got := Send[Size](objcID(box), RegisterName("sizeValue")); got != want {
				t.Fatalf("NSSize round-trip: got %+v, want %+v", got, want)
			}
		}},
		"NSRect": {check: func(t *testing.T) {
			want := Rect{Origin: Point{X: 10, Y: 20}, Size: Size{Width: 300, Height: 200}}
			box := SendClass("NSValue", "valueWithRect:", want)
			if got := Send[Rect](objcID(box), RegisterName("rectValue")); got != want {
				t.Fatalf("NSRect round-trip: got %+v, want %+v", got, want)
			}
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) { tt.check(t) })
	}
}

// TestExternPointer verifies that an extern NSString constant resolves to a
// non-nil object, which is how the graphical application reads AppKit symbols
// such as NSToolbarSpaceItemIdentifier.
func TestExternPointer(t *testing.T) {
	if err := LoadFramework(cocoaPath); err != nil {
		t.Skipf("Cocoa unavailable: %v", err)
	}
	tests := map[string]string{
		"run loop mode": "NSDefaultRunLoopMode",
		"toolbar space": "NSToolbarSpaceItemIdentifier",
		"foreground":    "NSForegroundColorAttributeName",
	}
	for name, symbol := range tests {
		t.Run(name, func(t *testing.T) {
			if ExternPointer(symbol) == nil {
				t.Fatalf("%s (%s) resolved to nil", name, symbol)
			}
		})
	}
}
