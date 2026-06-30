//go:build darwin && arm64
// +build darwin,arm64

package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// LinuxRosettaAvailability represents an availability of Rosetta support for Linux binaries.
//
//go:generate go run ./cmd/addtags -tags=darwin,arm64 -file linuxrosettaavailability_string_arm64.go stringer -type=LinuxRosettaAvailability -output=linuxrosettaavailability_string_arm64.go
type LinuxRosettaAvailability int

const (
	// LinuxRosettaAvailabilityNotSupported Rosetta support for Linux binaries is not available on the host system.
	LinuxRosettaAvailabilityNotSupported LinuxRosettaAvailability = iota

	// LinuxRosettaAvailabilityNotInstalled Rosetta support for Linux binaries is not installed on the host system.
	LinuxRosettaAvailabilityNotInstalled

	// LinuxRosettaAvailabilityInstalled Rosetta support for Linux is installed on the host system.
	LinuxRosettaAvailabilityInstalled
)

// LinuxRosettaDirectoryShare directory share to enable Rosetta support for Linux binaries.
// see: https://developer.apple.com/documentation/virtualization/vzlinuxrosettadirectoryshare?language=objc
type LinuxRosettaDirectoryShare struct {
	*pointer

	*baseDirectoryShare
}

var _ DirectoryShare = (*LinuxRosettaDirectoryShare)(nil)

// NewLinuxRosettaDirectoryShare creates a new Rosetta directory share if Rosetta support
// for Linux binaries is installed.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func NewLinuxRosettaDirectoryShare() (*LinuxRosettaDirectoryShare, error) {
	if err := macOSAvailable(13); err != nil {
		return nil, err
	}

	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)

	ds := &LinuxRosettaDirectoryShare{
		pointer: objc.NewPointer(
			objc.New("VZLinuxRosettaDirectoryShare", "initWithError:", errSlot),
		),
	}
	if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
		return nil, err
	}
	objc.SetFinalizer(ds, func(self *LinuxRosettaDirectoryShare) {
		objc.Release(self)
	})
	return ds, nil
}

// SetOptions enables translation caching and configure the socket communication type for Rosetta.
//
// This is only supported on macOS 14 and newer. Older versions do nothing.
func (ds *LinuxRosettaDirectoryShare) SetOptions(options LinuxRosettaCachingOptions) {
	if err := macOSAvailable(14); err != nil {
		return
	}
	objc.SendVoid(objc.Ptr(ds), "setOptions:", objc.Ptr(options))
}

// LinuxRosettaDirectoryShareInstallRosetta download and install Rosetta support
// for Linux binaries if necessary.
//
// This is only supported on macOS 13 and newer, error will
// be returned on older versions.
func LinuxRosettaDirectoryShareInstallRosetta() error {
	if err := macOSAvailable(13); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	block := objc.BlockError(func(errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			errCh <- err
		} else {
			errCh <- nil
		}
	})
	objc.ID(objc.GetClass("VZLinuxRosettaDirectoryShare")).Send(
		objc.RegisterName("installRosettaWithCompletionHandler:"),
		block,
	)
	err := <-errCh
	block.Release()
	return err
}

// LinuxRosettaDirectoryShareAvailability checks the availability of Rosetta support
// for the directory share.
//
// This is only supported on macOS 13 and newer, LinuxRosettaAvailabilityNotSupported will
// be returned on older versions.
func LinuxRosettaDirectoryShareAvailability() LinuxRosettaAvailability {
	if err := macOSAvailable(13); err != nil {
		return LinuxRosettaAvailabilityNotSupported
	}
	return LinuxRosettaAvailability(
		objc.Send[int32](
			objc.ID(objc.GetClass("VZLinuxRosettaDirectoryShare")),
			objc.RegisterName("availability"),
		),
	)
}

// LinuxRosettaCachingOptions for a directory sharing device configuration.
type LinuxRosettaCachingOptions interface {
	objc.NSObject

	linuxRosettaCachingOptions()
}

type baseLinuxRosettaCachingOptions struct{}

func (*baseLinuxRosettaCachingOptions) linuxRosettaCachingOptions() {}

// LinuxRosettaUnixSocketCachingOptions is an struct that represents caching options
// for a UNIX domain socket.
//
// This struct configures Rosetta to communicate with the Rosetta daemon using a UNIX domain socket.
type LinuxRosettaUnixSocketCachingOptions struct {
	*pointer

	*baseLinuxRosettaCachingOptions
}

var _ LinuxRosettaCachingOptions = (*LinuxRosettaUnixSocketCachingOptions)(nil)

// NewLinuxRosettaUnixSocketCachingOptions creates a new Rosetta caching options object for
// a UNIX domain socket with the path you specify.
//
// The path of the Unix Domain Socket to be used to communicate with the Rosetta translation daemon.
//
// This is only supported on macOS 14 and newer, error will
// be returned on older versions.
func NewLinuxRosettaUnixSocketCachingOptions(path string) (*LinuxRosettaUnixSocketCachingOptions, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}
	maxPathLen := maximumPathLengthLinuxRosettaUnixSocketCachingOptions()
	if maxPathLen < len(path) {
		return nil, fmt.Errorf("path length exceeds maximum allowed length of %d", maxPathLen)
	}

	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)

	usco := &LinuxRosettaUnixSocketCachingOptions{
		pointer: objc.NewPointer(
			objc.New("VZLinuxRosettaUnixSocketCachingOptions", "initWithPath:error:",
				objc.NSString(path), errSlot),
		),
	}
	if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
		return nil, err
	}
	objc.SetFinalizer(usco, func(self *LinuxRosettaUnixSocketCachingOptions) {
		objc.Release(self)
	})
	return usco, nil
}

func maximumPathLengthLinuxRosettaUnixSocketCachingOptions() int {
	return int(objc.Send[uint32](
		objc.ID(objc.GetClass("VZLinuxRosettaUnixSocketCachingOptions")),
		objc.RegisterName("maximumPathLength"),
	))
}

// LinuxRosettaAbstractSocketCachingOptions is caching options for an abstract socket.
//
// Use this object to configure Rosetta to communicate with the Rosetta daemon using an abstract socket.
type LinuxRosettaAbstractSocketCachingOptions struct {
	*pointer

	*baseLinuxRosettaCachingOptions
}

var _ LinuxRosettaCachingOptions = (*LinuxRosettaAbstractSocketCachingOptions)(nil)

// NewLinuxRosettaAbstractSocketCachingOptions creates a new LinuxRosettaAbstractSocketCachingOptions.
//
// The name of the Abstract Socket to be used to communicate with the Rosetta translation daemon.
//
// This is only supported on macOS 14 and newer, error will
// be returned on older versions.
func NewLinuxRosettaAbstractSocketCachingOptions(name string) (*LinuxRosettaAbstractSocketCachingOptions, error) {
	if err := macOSAvailable(14); err != nil {
		return nil, err
	}
	maxNameLen := maximumNameLengthVZLinuxRosettaAbstractSocketCachingOptions()
	if maxNameLen < len(name) {
		return nil, fmt.Errorf("name length exceeds maximum allowed length of %d", maxNameLen)
	}

	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)

	asco := &LinuxRosettaAbstractSocketCachingOptions{
		pointer: objc.NewPointer(
			objc.New("VZLinuxRosettaAbstractSocketCachingOptions", "initWithName:error:",
				objc.NSString(name), errSlot),
		),
	}
	if err := newNSError(objc.ErrorFromSlot(errSlot)); err != nil {
		return nil, err
	}
	objc.SetFinalizer(asco, func(self *LinuxRosettaAbstractSocketCachingOptions) {
		objc.Release(self)
	})
	return asco, nil
}

func maximumNameLengthVZLinuxRosettaAbstractSocketCachingOptions() int {
	return int(objc.Send[uint32](
		objc.ID(objc.GetClass("VZLinuxRosettaAbstractSocketCachingOptions")),
		objc.RegisterName("maximumNameLength"),
	))
}
