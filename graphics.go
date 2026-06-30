package vz

import (
	"github.com/Code-Hex/vz/v3/internal/objc"
)

// GraphicsDeviceConfiguration is an interface for a graphics device configuration.
type GraphicsDeviceConfiguration interface {
	objc.NSObject

	graphicsDeviceConfiguration()
}

type baseGraphicsDeviceConfiguration struct{}

func (*baseGraphicsDeviceConfiguration) graphicsDeviceConfiguration() {}

// VirtioGraphicsDeviceConfiguration is configuration that represents the configuration
// of a Virtio graphics device for a Linux VM.
//
// This device configuration creates a graphics device using paravirtualization.
// The emulated device follows the Virtio GPU Device specification.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiographicsdeviceconfiguration?language=objc
type VirtioGraphicsDeviceConfiguration struct {
	*pointer

	*baseGraphicsDeviceConfiguration
}

var _ GraphicsDeviceConfiguration = (*VirtioGraphicsDeviceConfiguration)(nil)

// NewVirtioGraphicsDeviceConfiguration creates a new Virtio graphics device.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewVirtioGraphicsDeviceConfiguration() (*VirtioGraphicsDeviceConfiguration, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}
	graphicsConfiguration := &VirtioGraphicsDeviceConfiguration{
		pointer: objc.NewPointer(
			objc.New("VZVirtioGraphicsDeviceConfiguration", "init"),
		),
	}
	objc.SetFinalizer(graphicsConfiguration, func(self *VirtioGraphicsDeviceConfiguration) {
		objc.Release(self)
	})
	return graphicsConfiguration, nil
}

// SetScanouts sets the displays associated with this graphics device.
//
// Maximum of one scanout is supported.
func (v *VirtioGraphicsDeviceConfiguration) SetScanouts(scanoutConfigs ...*VirtioGraphicsScanoutConfiguration) {
	ptrs := make([]objc.NSObject, len(scanoutConfigs))
	for i, val := range scanoutConfigs {
		ptrs[i] = val
	}
	array := objc.ConvertToNSMutableArray(ptrs)
	objc.SendVoid(objc.Ptr(v), "setScanouts:", objc.SendPtr(objc.Ptr(array), "copy"))
}

// VirtioGraphicsScanoutConfiguration is the configuration for a Virtio graphics device
// that configures the dimensions of the graphics device for a Linux VM.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiographicsscanoutconfiguration?language=objc
type VirtioGraphicsScanoutConfiguration struct {
	*pointer
}

// NewVirtioGraphicsScanoutConfiguration creates a Virtio graphics device with the specified dimensions.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewVirtioGraphicsScanoutConfiguration(widthInPixels, heightInPixels int64) (*VirtioGraphicsScanoutConfiguration, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}

	graphicsScanoutConfiguration := &VirtioGraphicsScanoutConfiguration{
		pointer: objc.NewPointer(
			objc.New(
				"VZVirtioGraphicsScanoutConfiguration",
				"initWithWidthInPixels:heightInPixels:",
				widthInPixels,
				heightInPixels,
			),
		),
	}
	objc.SetFinalizer(graphicsScanoutConfiguration, func(self *VirtioGraphicsScanoutConfiguration) {
		objc.Release(self)
	})
	return graphicsScanoutConfiguration, nil
}
