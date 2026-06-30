package vz

import (
	"os"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// DirectorySharingDeviceConfiguration for a directory sharing device configuration.
type DirectorySharingDeviceConfiguration interface {
	objc.NSObject

	directorySharingDeviceConfiguration()
}

type baseDirectorySharingDeviceConfiguration struct{}

func (*baseDirectorySharingDeviceConfiguration) directorySharingDeviceConfiguration() {}

var _ DirectorySharingDeviceConfiguration = (*VirtioFileSystemDeviceConfiguration)(nil)

// VirtioFileSystemDeviceConfiguration is a configuration of a Virtio file system device.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiofilesystemdeviceconfiguration?language=objc
type VirtioFileSystemDeviceConfiguration struct {
	*pointer

	*baseDirectorySharingDeviceConfiguration
}

// NewVirtioFileSystemDeviceConfiguration create a new VirtioFileSystemDeviceConfiguration.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewVirtioFileSystemDeviceConfiguration(tag string) (*VirtioFileSystemDeviceConfiguration, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)

	nsTag := objc.NSString(tag)
	class := objc.GetClass("VZVirtioFileSystemDeviceConfiguration")
	var ptr unsafe.Pointer
	if objc.Send[bool](objc.ID(class), objc.RegisterName("validateTag:error:"), nsTag, errSlot) {
		ptr = objc.New("VZVirtioFileSystemDeviceConfiguration", "initWithTag:", nsTag)
	}
	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: objc.NewPointer(ptr),
	}
	if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
		return nil, err
	}
	objc.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
		objc.Release(self)
	})
	return fsdConfig, nil
}

// SetDirectoryShare sets the directory share associated with this configuration.
func (c *VirtioFileSystemDeviceConfiguration) SetDirectoryShare(share DirectoryShare) {
	objc.SendVoid(objc.Ptr(c), "setShare:", objc.Ptr(share))
}

// SharedDirectory is a shared directory.
type SharedDirectory struct {
	*pointer
}

// NewSharedDirectory creates a new shared directory.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewSharedDirectory(dirPath string, readOnly bool) (*SharedDirectory, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dirPath); err != nil {
		return nil, err
	}

	sd := &SharedDirectory{
		pointer: objc.NewPointer(
			objc.New("VZSharedDirectory", "initWithURL:readOnly:", objc.FileURL(dirPath), readOnly),
		),
	}
	objc.SetFinalizer(sd, func(self *SharedDirectory) {
		objc.Release(self)
	})
	return sd, nil
}

// DirectoryShare is the base interface for a directory share.
type DirectoryShare interface {
	objc.NSObject

	directoryShare()
}

type baseDirectoryShare struct{}

func (*baseDirectoryShare) directoryShare() {}

var _ DirectoryShare = (*SingleDirectoryShare)(nil)

// SingleDirectoryShare defines the directory share for a single directory.
type SingleDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

// NewSingleDirectoryShare creates a new single directory share.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewSingleDirectoryShare(share *SharedDirectory) (*SingleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	config := &SingleDirectoryShare{
		pointer: objc.NewPointer(
			objc.New("VZSingleDirectoryShare", "initWithDirectory:", objc.Ptr(share)),
		),
	}
	objc.SetFinalizer(config, func(self *SingleDirectoryShare) {
		objc.Release(self)
	})
	return config, nil
}

// MultipleDirectoryShare defines the directory share for multiple directories.
type MultipleDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*MultipleDirectoryShare)(nil)

// NewMultipleDirectoryShare creates a new multiple directories share.
//
// This is only supported on macOS 12 and newer, error will
// be returned on older versions.
func NewMultipleDirectoryShare(shares map[string]*SharedDirectory) (*MultipleDirectoryShare, error) {
	if err := macOSAvailable(12); err != nil {
		return nil, err
	}
	directories := make(map[string]objc.NSObject, len(shares))
	for k, v := range shares {
		directories[k] = v
	}

	dict := objc.ConvertToNSMutableDictionary(directories)

	config := &MultipleDirectoryShare{
		pointer: objc.NewPointer(
			objc.New("VZMultipleDirectoryShare", "initWithDirectories:", objc.Ptr(dict)),
		),
	}
	objc.SetFinalizer(config, func(self *MultipleDirectoryShare) {
		objc.Release(self)
	})
	return config, nil
}

// MacOSGuestAutomountTag returns the macOS automount tag.
//
// A device configured with this tag will be automatically mounted in a macOS guest.
// This is only supported on macOS 13 and newer, error will be returned on older versions.
func MacOSGuestAutomountTag() (string, error) {
	if err := macOSAvailable(13); err != nil {
		return "", err
	}
	class := objc.GetClass("VZVirtioFileSystemDeviceConfiguration")
	tag := objc.Send[unsafe.Pointer](objc.ID(class), objc.RegisterName("macOSGuestAutomountTag"))
	return objc.Send[string](objc.ID(uintptr(tag)), objc.RegisterName("UTF8String")), nil
}
