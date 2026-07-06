package vz

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestCustomVirtioSupportsSaveRestore verifies the supportsSaveRestore flag defaults to
// false and round-trips through the Objective-C object.
func TestCustomVirtioSupportsSaveRestore(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	tests := map[string]struct {
		set bool
	}{
		"success: enabled":  {set: true},
		"success: disabled": {set: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := NewCustomVirtioDeviceConfiguration()
			if err != nil {
				t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
			}
			if config.SupportsSaveRestore() {
				t.Error("SupportsSaveRestore defaulted to true, want false")
			}
			config.SetSupportsSaveRestore(tt.set)
			if got := config.SupportsSaveRestore(); got != tt.set {
				t.Errorf("SupportsSaveRestore = %v, want %v", got, tt.set)
			}
		})
	}
}

// TestCustomVirtioSaveRestoreIMPs drives the two return-valued save/restore delegate
// methods on the real Go-backed delegate, on the device's serial queue, and asserts they
// dispatch to the handler and marshal their returns correctly.
//
// This is headless: no virtual machine has been created, so the device the framework
// would pass is nil (the handlers ignore it) and the methods are invoked directly rather
// than by the framework. It proves the selectors are registered, the handler callbacks
// run, the NSData* save-state return round-trips (with a nil/absent handler yielding a
// valid empty NSData, never nil), the BOOL restore return round-trips, and the +0
// saveState argument is copied into the handler's slice. The actual save/restore of a
// running VM — and the framework's +0 ownership of the returned NSData — is a documented
// manual e2e.
func TestCustomVirtioSaveRestoreIMPs(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	tests := map[string]struct {
		save        func(device *CustomVirtioDevice) []byte
		hasRestore  bool
		restoreRet  bool
		saveStateIn []byte
		wantSaved   []byte
		wantRestore bool
	}{
		"no handlers: empty save, restore false": {
			save:        nil,
			hasRestore:  false,
			saveStateIn: nil,
			wantSaved:   []byte{},
			wantRestore: false,
		},
		"save bytes, restore true": {
			save:        func(*CustomVirtioDevice) []byte { return []byte{0x01, 0x02, 0x03} },
			hasRestore:  true,
			restoreRet:  true,
			saveStateIn: []byte{0x09, 0x09},
			wantSaved:   []byte{0x01, 0x02, 0x03},
			wantRestore: true,
		},
		"save empty, restore false": {
			save:        func(*CustomVirtioDevice) []byte { return []byte{} },
			hasRestore:  true,
			restoreRet:  false,
			saveStateIn: []byte{},
			wantSaved:   []byte{},
			wantRestore: false,
		},
		"save nil bytes handed over as empty": {
			save:        func(*CustomVirtioDevice) []byte { return nil },
			hasRestore:  false,
			saveStateIn: nil,
			wantSaved:   []byte{},
			wantRestore: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := NewCustomVirtioDeviceConfiguration()
			if err != nil {
				t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
			}

			var gotRestoreArg []byte
			var restoreSeen bool
			h := CustomVirtioHandler{SaveStateForRestore: tt.save}
			if tt.hasRestore {
				h.ShouldRestore = func(_ *CustomVirtioDevice, saveState []byte) bool {
					gotRestoreArg = saveState
					restoreSeen = true
					return tt.restoreRet
				}
			}
			config.SetHandler(h)

			var gotSaved []byte
			var gotRestore bool
			objc.DispatchSync(config.queue, func() {
				data := objc.Send[unsafe.Pointer](
					objc.ID(uintptr(config.delegate)),
					objc.RegisterName("customVirtioDeviceSaveStateForRestore:"),
					unsafe.Pointer(nil),
				)
				gotSaved = objc.NSDataToBytes(data)

				// A +1 NSData standing in for the framework-vended saveState argument.
				sd := objc.NSData(tt.saveStateIn)
				gotRestore = objc.Send[bool](
					objc.ID(uintptr(config.delegate)),
					objc.RegisterName("customVirtioDeviceShouldRestore:saveState:"),
					unsafe.Pointer(nil), sd,
				)
				objc.SendVoid(sd, "release")
			})

			if gotSaved == nil {
				t.Error("SaveStateForRestore returned nil bytes; the IMP must return an empty NSData, not a nil pointer")
			}
			if !bytes.Equal(gotSaved, tt.wantSaved) {
				t.Errorf("SaveStateForRestore round-trip = %v, want %v", gotSaved, tt.wantSaved)
			}
			if gotRestore != tt.wantRestore {
				t.Errorf("ShouldRestore return = %v, want %v", gotRestore, tt.wantRestore)
			}
			if tt.hasRestore {
				if !restoreSeen {
					t.Error("ShouldRestore handler was not invoked")
				}
				wantArg := tt.saveStateIn
				if wantArg == nil {
					wantArg = []byte{}
				}
				if !bytes.Equal(gotRestoreArg, wantArg) {
					t.Errorf("ShouldRestore saveState arg = %v, want %v (the +0 NSData must be copied through)", gotRestoreArg, wantArg)
				}
			}
		})
	}
}

// TestCustomVirtioSaveRestoreValidateFinding validates an EFI base configuration with a
// custom Virtio device that enables supportsSaveRestore and installs a handler, and logs
// the outcome. With both save/restore selectors always registered on the Go-backed
// delegate, enabling the flag must not trip the framework's "delegate must implement the
// save/restore methods" check. Neither Validate call must panic. Under make test the
// binary is signed, so Validate exercises the real framework path.
func TestCustomVirtioSaveRestoreValidateFinding(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	bl, err := NewEFIBootLoader()
	if err != nil {
		t.Fatalf("NewEFIBootLoader: %v", err)
	}
	config, err := NewVirtualMachineConfiguration(bl, 1, 256*1024*1024)
	if err != nil {
		t.Fatalf("NewVirtualMachineConfiguration: %v", err)
	}
	cvdc, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	cvdc.SetDeviceID(0x1AF4)
	cvdc.SetPCIClassID(0x02)
	cvdc.SetPCISubclassID(0x00)
	cvdc.SetVirtioQueueCount(1)
	cvdc.SetSupportsSaveRestore(true)
	cvdc.SetHandler(CustomVirtioHandler{
		SaveStateForRestore: func(*CustomVirtioDevice) []byte { return []byte{} },
		ShouldRestore:       func(*CustomVirtioDevice, []byte) bool { return true },
	})
	config.SetCustomVirtioDevicesVirtualMachineConfiguration(
		[]*CustomVirtioDeviceConfiguration{cvdc},
	)
	ok, err := config.Validate()
	t.Logf("supportsSaveRestore=true + handler -> Validate() ok=%v err=%v", ok, err)
}
