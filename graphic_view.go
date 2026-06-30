package vz

/*
#cgo darwin CFLAGS: -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization -framework Cocoa
#include <stdlib.h>
# include "virtualization_12.h"
*/
import "C"

import (
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// This file is the sole remaining cgo island in the package. Everything else is
// implemented with purego; only the AppKit/Cocoa window+runloop entrypoint below
// still goes through cgo because it must drive NSApplication on the main thread.
// It is deliberately isolated here so it can be migrated last (see the migration
// plan, Wave 5) without holding back the rest of the cgo-free conversion.

type startGraphicApplicationOptions struct {
	title            string
	enableController bool
}

// StartGraphicApplicationOption is an option for display graphics start.
type StartGraphicApplicationOption func(*startGraphicApplicationOptions) error

// WithWindowTitle is an option to set window title of display graphics window.
func WithWindowTitle(title string) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.title = title
		return nil
	}
}

// WithController is an option to set virtual machine controller on graphics window toolbar.
func WithController(enable bool) StartGraphicApplicationOption {
	return func(sgao *startGraphicApplicationOptions) error {
		sgao.enableController = enable
		return nil
	}
}

// StartGraphicApplication starts an application to display graphics of the VM.
//
// You must to call runtime.LockOSThread before calling this method.
//
// This is only supported on macOS 12 and newer, error will be returned on older versions.
func (v *VirtualMachine) StartGraphicApplication(width, height float64, opts ...StartGraphicApplicationOption) error {
	if err := macOSAvailable(12); err != nil {
		return err
	}
	defaultOpts := &startGraphicApplicationOptions{}
	for _, opt := range opts {
		if err := opt(defaultOpts); err != nil {
			return err
		}
	}
	title := C.CString(defaultOpts.title)
	defer C.free(unsafe.Pointer(title))
	C.startVirtualMachineWindow(
		objc.Ptr(v),
		v.dispatchQueue,
		C.double(width),
		C.double(height),
		title,
		C.bool(defaultOpts.enableController),
	)
	return nil
}
