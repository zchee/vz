//go:build darwin && arm64
// +build darwin,arm64

package vz

import "github.com/Code-Hex/vz/v3/internal/objc"

// ValidateSaveRestoreSupport Determines whether the framework can save or restore the VM’s current configuration.
//
// Verify that a virtual machine with this configuration is savable.
// Not all configuration options can be safely saved and restored from file.
//
// If this evaluates to false, the caller should expect future calls to `(*VirtualMachine).SaveMachineStateToPath` to fail.
// error If not nil, assigned with an error describing the unsupported configuration option.
func (v *VirtualMachineConfiguration) ValidateSaveRestoreSupport() (bool, error) {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	ret := objc.Send[bool](
		objc.ID(uintptr(objc.Ptr(v))),
		objc.RegisterName("validateSaveRestoreSupportWithError:"),
		errSlot,
	)
	if objc.HasError(errSlot) {
		return false, newNSError(objc.ErrorFromSlot(errSlot))
	}
	return ret, nil
}
