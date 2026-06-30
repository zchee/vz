package vz

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// SerialPortAttachment interface for a serial port attachment.
//
// A serial port attachment defines how the virtual machine's serial port interfaces with the host system.
type SerialPortAttachment interface {
	objc.NSObject

	serialPortAttachment()
}

type baseSerialPortAttachment struct{}

func (*baseSerialPortAttachment) serialPortAttachment() {}

var _ SerialPortAttachment = (*FileHandleSerialPortAttachment)(nil)

// FileHandleSerialPortAttachment defines a serial port attachment from a file handle.
//
// Data written to fileHandleForReading goes to the guest. Data sent from the guest appears on fileHandleForWriting.
// see: https://developer.apple.com/documentation/virtualization/vzfilehandleserialportattachment?language=objc
type FileHandleSerialPortAttachment struct {
	*pointer

	*baseSerialPortAttachment
}

// NewFileHandleSerialPortAttachment initialize the FileHandleSerialPortAttachment from file handles.
//
// read parameter is an *os.File for reading from the file.
// write parameter is an *os.File for writing to the file.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewFileHandleSerialPortAttachment(read, write *os.File) (*FileHandleSerialPortAttachment, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	readHandle, err := newFileHandleDupFd(int(read.Fd()))
	if err != nil {
		return nil, err
	}
	writeHandle, err := newFileHandleDupFd(int(write.Fd()))
	if err != nil {
		return nil, err
	}

	attachment := &FileHandleSerialPortAttachment{
		pointer: objc.NewPointer(
			objc.New(
				"VZFileHandleSerialPortAttachment",
				"initWithFileHandleForReading:fileHandleForWriting:",
				readHandle,
				writeHandle,
			),
		),
	}
	objc.SetFinalizer(attachment, func(self *FileHandleSerialPortAttachment) {
		objc.Release(self)
	})
	return attachment, nil
}

var _ SerialPortAttachment = (*FileSerialPortAttachment)(nil)

// FileSerialPortAttachment defines a serial port attachment from a file.
//
// Any data sent by the guest on the serial interface is written to the file.
// No data is sent to the guest over serial with this attachment.
// see: https://developer.apple.com/documentation/virtualization/vzfileserialportattachment?language=objc
type FileSerialPortAttachment struct {
	*pointer

	*baseSerialPortAttachment
}

// NewFileSerialPortAttachment initialize the FileSerialPortAttachment from a path of a file.
// If error is not nil, used to report errors if intialization fails.
//
//   - path of the file for the attachment on the local file system.
//   - shouldAppend True if the file should be opened in append mode, false otherwise.
//     When a file is opened in append mode, writing to that file will append to the end of it.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewFileSerialPortAttachment(path string, shouldAppend bool) (*FileSerialPortAttachment, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)

	attachment := &FileSerialPortAttachment{
		pointer: objc.NewPointer(
			objc.New(
				"VZFileSerialPortAttachment",
				"initWithURL:append:error:",
				objc.FileURL(path),
				shouldAppend,
				errSlot,
			),
		),
	}
	if objc.HasError(errSlot) {
		return nil, newNSError(objc.ErrorFromSlot(errSlot))
	}
	objc.SetFinalizer(attachment, func(self *FileSerialPortAttachment) {
		objc.Release(self)
	})
	return attachment, nil
}

// VirtioConsoleDeviceSerialPortConfiguration represents Virtio Console Serial Port Device.
//
// The device creates a console which enables communication between the host and the guest through the Virtio interface.
// The device sets up a single port on the Virtio console device.
// see: https://developer.apple.com/documentation/virtualization/vzvirtioconsoledeviceserialportconfiguration?language=objc
type VirtioConsoleDeviceSerialPortConfiguration struct {
	*pointer
}

// NewVirtioConsoleDeviceSerialPortConfiguration creates a new NewVirtioConsoleDeviceSerialPortConfiguration.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtioConsoleDeviceSerialPortConfiguration(attachment SerialPortAttachment) (*VirtioConsoleDeviceSerialPortConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	ptr := objc.New("VZVirtioConsoleDeviceSerialPortConfiguration", "init")
	objc.SendVoid(ptr, "setAttachment:", objc.Ptr(attachment))
	config := &VirtioConsoleDeviceSerialPortConfiguration{
		pointer: objc.NewPointer(ptr),
	}
	objc.SetFinalizer(config, func(self *VirtioConsoleDeviceSerialPortConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

// newFileHandleDupFd duplicates fd and wraps it in an NSFileHandle that closes
// the duplicate when it is deallocated. It mirrors the Objective-C
// newFileHandleDupFd helper: dup(2) the descriptor so the NSFileHandle owns an
// independent copy, then -[NSFileHandle initWithFileDescriptor:closeOnDealloc:].
func newFileHandleDupFd(fd int) (unsafe.Pointer, error) {
	dupedFd, err := syscall.Dup(fd)
	if err != nil {
		return nil, os.NewSyscallError("dup", err)
	}
	return objc.New(
		"NSFileHandle",
		"initWithFileDescriptor:closeOnDealloc:",
		dupedFd,
		true,
	), nil
}
