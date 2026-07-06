package vz

import (
	"testing"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

func TestVirtioSharedMemoryClassesResolve(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("shared memory requires macOS 27+: %v", err)
	}
	for _, name := range []string{"VZVirtioSharedMemoryRegionConfiguration", "VZVirtioSharedMemoryRegion"} {
		if objc.ID(objc.GetClass(name)) == 0 {
			t.Errorf("%s class did not resolve", name)
		}
	}
}

func TestVirtioSharedMemoryRegionConfiguration(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("shared memory requires macOS 27+: %v", err)
	}
	tests := map[string]struct {
		regionID uint8
		size     uint64
	}{
		"success: page":   {regionID: 0, size: 4096},
		"success: larger": {regionID: 7, size: 16 * 1024 * 1024},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c, err := NewVirtioSharedMemoryRegionConfiguration(tt.regionID, tt.size)
			if err != nil {
				t.Fatalf("NewVirtioSharedMemoryRegionConfiguration: %v", err)
			}
			if got := c.RegionID(); got != tt.regionID {
				t.Errorf("RegionID = %d, want %d", got, tt.regionID)
			}
			if got := c.Size(); got != tt.size {
				t.Errorf("Size = %d, want %d", got, tt.size)
			}
		})
	}
}

func TestMaximumAllowedSharedMemoryRegionCount(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("shared memory requires macOS 27+: %v", err)
	}
	n, err := MaximumAllowedSharedMemoryRegionCount()
	if err != nil {
		t.Fatalf("MaximumAllowedSharedMemoryRegionCount: %v", err)
	}
	if n == 0 {
		t.Fatal("want a positive maximum shared-memory region count")
	}
	t.Logf("maximumAllowedSharedMemoryRegionCount = %d", n)
}

func TestCustomVirtioSetSharedMemoryRegions(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("shared memory requires macOS 27+: %v", err)
	}
	config, err := NewCustomVirtioDeviceConfiguration()
	if err != nil {
		t.Fatalf("NewCustomVirtioDeviceConfiguration: %v", err)
	}
	maxN, err := MaximumAllowedSharedMemoryRegionCount()
	if err != nil {
		t.Fatalf("MaximumAllowedSharedMemoryRegionCount: %v", err)
	}
	n := 2
	if uint(n) > maxN {
		n = int(maxN)
	}
	regions := make([]*VirtioSharedMemoryRegionConfiguration, n)
	for i := range regions {
		r, err := NewVirtioSharedMemoryRegionConfiguration(uint8(i), 4096)
		if err != nil {
			t.Fatal(err)
		}
		regions[i] = r
	}
	config.SetSharedMemoryRegions(regions)

	got := len(objc.NewNSArray(objc.SendPtr(objc.Ptr(config), "sharedMemoryRegions")).ToPointerSlice())
	if got != n {
		t.Errorf("sharedMemoryRegions round-trip count = %d, want %d", got, n)
	}
}

// TestVirtioSharedMemoryValidateFinding attaches a shared-memory-equipped custom Virtio
// device to an EFI VM configuration and validates it. Under `make test` (signed) this is
// a real Validate; under plain `go test` it short-circuits on the virtualization
// entitlement (see TestCustomVirtioValidateFinding). The outcome is logged; the call must
// not panic.
func TestVirtioSharedMemoryValidateFinding(t *testing.T) {
	if err := macOSAvailable(27); err != nil {
		t.Skipf("shared memory requires macOS 27+: %v", err)
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
	region, err := NewVirtioSharedMemoryRegionConfiguration(0, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	cvdc.SetSharedMemoryRegions([]*VirtioSharedMemoryRegionConfiguration{region})
	config.SetCustomVirtioDevicesVirtualMachineConfiguration([]*CustomVirtioDeviceConfiguration{cvdc})

	ok, verr := config.Validate()
	t.Logf("shared-memory Validate() -> ok=%v err=%v", ok, verr)
}
