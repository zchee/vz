package vz

import (
	"bytes"
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// TestCustomVirtioClassesResolve verifies the custom Virtio configuration classes
// resolve on a macOS 27 host.
func TestCustomVirtioClassesResolve(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	for _, name := range []string{
		"VZCustomVirtioDeviceConfiguration",
		"VZVirtioFeatureSet",
		"VZVirtioDeviceSpecificConfiguration",
	} {
		if objc.ID(objc.GetClass(name)) == 0 {
			t.Errorf("%s class did not resolve", name)
		}
	}
}

// TestCustomVirtioDeviceConfigurationProperties verifies the scalar discovery
// properties round-trip through the Objective-C object.
func TestCustomVirtioDeviceConfigurationProperties(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	tests := map[string]struct {
		set  func(*CustomVirtioDeviceConfiguration)
		get  func(*CustomVirtioDeviceConfiguration) uint64
		want uint64
	}{
		"success: deviceID": {
			set:  func(c *CustomVirtioDeviceConfiguration) { c.SetDeviceID(0x1234) },
			get:  func(c *CustomVirtioDeviceConfiguration) uint64 { return uint64(c.DeviceID()) },
			want: 0x1234,
		},
		"success: PCIClassID": {
			set:  func(c *CustomVirtioDeviceConfiguration) { c.SetPCIClassID(0x02) },
			get:  func(c *CustomVirtioDeviceConfiguration) uint64 { return uint64(c.PCIClassID()) },
			want: 0x02,
		},
		"success: PCISubclassID": {
			set:  func(c *CustomVirtioDeviceConfiguration) { c.SetPCISubclassID(0x80) },
			get:  func(c *CustomVirtioDeviceConfiguration) uint64 { return uint64(c.PCISubclassID()) },
			want: 0x80,
		},
		"success: virtioQueueCount": {
			set:  func(c *CustomVirtioDeviceConfiguration) { c.SetVirtioQueueCount(4) },
			get:  func(c *CustomVirtioDeviceConfiguration) uint64 { return uint64(c.VirtioQueueCount()) },
			want: 4,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := NewCustomVirtioDeviceConfiguration()
			if err != nil {
				t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
			}
			tt.set(config)
			if got := tt.get(config); got != tt.want {
				t.Errorf("round-trip = %#x, want %#x", got, tt.want)
			}
		})
	}
}

// TestVirtioFeatureSet verifies the mandatory and optional feature sets round-trip
// their subsets, and that mutations persist on re-fetch — proving the returned
// object is the configuration's own feature set, not a copy.
func TestVirtioFeatureSet(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	const (
		wantSubset0 uint32 = 0xDEADBEEF
		wantSubset1 uint32 = 0x0BADF00D
	)
	tests := map[string]struct {
		features func(*CustomVirtioDeviceConfiguration) *VirtioFeatureSet
	}{
		"success: mandatory": {features: (*CustomVirtioDeviceConfiguration).MandatoryFeatures},
		"success: optional":  {features: (*CustomVirtioDeviceConfiguration).OptionalFeatures},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := NewCustomVirtioDeviceConfiguration()
			if err != nil {
				t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
			}
			fs := tt.features(config)
			// The framework force-sets some default feature bits (for example the
			// optional set enables a few bits for performance), so capture that
			// baseline first and assert our bits are applied on top of it.
			fs.SetSubset0(0)
			fs.SetSubset1(0)
			base0, base1 := fs.Subset0(), fs.Subset1()
			fs.SetSubset0(wantSubset0)
			fs.SetSubset1(wantSubset1)
			want0, want1 := wantSubset0|base0, wantSubset1|base1
			if got := fs.Subset0(); got != want0 {
				t.Errorf("Subset0 = %#x, want %#x", got, want0)
			}
			if got := fs.Subset1(); got != want1 {
				t.Errorf("Subset1 = %#x, want %#x", got, want1)
			}
			// Re-fetching must return the same values, proving the feature set is
			// the configuration's own object rather than a copy.
			refetched := tt.features(config)
			if got := refetched.Subset0(); got != want0 {
				t.Errorf("re-fetched Subset0 = %#x, want %#x (mutation did not persist)", got, want0)
			}
			if got := refetched.Subset1(); got != want1 {
				t.Errorf("re-fetched Subset1 = %#x, want %#x (mutation did not persist)", got, want1)
			}
		})
	}
}

// TestVirtioDeviceSpecificConfiguration verifies the configuration-data bytes
// round-trip through NSData for empty, small, and larger payloads, and that the
// result can be attached to a device configuration.
func TestVirtioDeviceSpecificConfiguration(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	tests := map[string]struct {
		data []byte
	}{
		"success: empty":  {data: []byte{}},
		"success: small":  {data: []byte{0x01, 0x02, 0x03, 0x04}},
		"success: larger": {data: bytes.Repeat([]byte{0xAB}, 512)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dsc, err := NewVirtioDeviceSpecificConfiguration(tt.data)
			if err != nil {
				t.Fatalf("NewVirtioDeviceSpecificConfiguration: %v", err)
			}
			if got := dsc.ConfigurationData(); !bytes.Equal(tt.data, got) {
				t.Errorf("configurationData round-trip = %v, want %v", got, tt.data)
			}
			config, err := NewCustomVirtioDeviceConfiguration()
			if err != nil {
				t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
			}
			config.SetDeviceSpecificConfiguration(dsc)
		})
	}
}

// TestCustomVirtioValidateFinding resolves plan Open Q1: does a provider-less custom
// Virtio configuration validate? It validates an EFI base configuration, then the
// same base with one custom Virtio device attached, and logs both outcomes so the
// finding is recorded. Neither Validate call must panic.
func TestCustomVirtioValidateFinding(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("custom Virtio requires macOS 27+: %v", err)
	}
	newBase := func(t *testing.T) *VirtualMachineConfiguration {
		t.Helper()
		bl, err := NewEFIBootLoader()
		if err != nil {
			t.Fatalf("NewEFIBootLoader: %v", err)
		}
		config, err := NewVirtualMachineConfiguration(bl, 1, 256*1024*1024)
		if err != nil {
			t.Fatalf("NewVirtualMachineConfiguration: %v", err)
		}
		return config
	}
	tests := map[string]struct {
		attachCustom bool
	}{
		"base config (no custom virtio)":     {attachCustom: false},
		"base config + custom virtio device": {attachCustom: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config := newBase(t)
			if tt.attachCustom {
				cvdc, err := NewCustomVirtioDeviceConfiguration()
				if err != nil {
					t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
				}
				cvdc.SetDeviceID(0x1AF4)
				cvdc.SetPCIClassID(0x02)
				cvdc.SetPCISubclassID(0x00)
				cvdc.SetVirtioQueueCount(1)
				config.SetCustomVirtioDevicesVirtualMachineConfiguration(
					[]*CustomVirtioDeviceConfiguration{cvdc},
				)
			}
			ok, err := config.Validate()
			t.Logf("Open Q1 finding: attachCustom=%v -> Validate() ok=%v err=%v", tt.attachCustom, ok, err)
		})
	}
}
