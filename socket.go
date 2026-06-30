package vz

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// SocketDeviceConfiguration for a socket device configuration.
type SocketDeviceConfiguration interface {
	objc.NSObject

	socketDeviceConfiguration()
}

type baseSocketDeviceConfiguration struct{}

func (*baseSocketDeviceConfiguration) socketDeviceConfiguration() {}

var _ SocketDeviceConfiguration = (*VirtioSocketDeviceConfiguration)(nil)

// VirtioSocketDeviceConfiguration is a configuration of the Virtio socket device.
//
// This configuration creates a Virtio socket device for the guest which communicates with the host through the Virtio interface.
// Only one Virtio socket device can be used per virtual machine.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdeviceconfiguration?language=objc
type VirtioSocketDeviceConfiguration struct {
	*pointer

	*baseSocketDeviceConfiguration
}

// NewVirtioSocketDeviceConfiguration creates a new VirtioSocketDeviceConfiguration.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func NewVirtioSocketDeviceConfiguration() (*VirtioSocketDeviceConfiguration, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	config := newVirtioSocketDeviceConfiguration(
		objc.New("VZVirtioSocketDeviceConfiguration", "init"),
	)

	objc.SetFinalizer(config, func(self *VirtioSocketDeviceConfiguration) {
		objc.Release(self)
	})
	return config, nil
}

func newVirtioSocketDeviceConfiguration(ptr unsafe.Pointer) *VirtioSocketDeviceConfiguration {
	return &VirtioSocketDeviceConfiguration{
		pointer: objc.NewPointer(ptr),
	}
}

// VirtioSocketDevice a device that manages port-based connections between the guest system and the host computer.
//
// Don’t create a VirtioSocketDevice struct directly. Instead, when you request a socket device in your configuration,
// the virtual machine creates it and you can get it via SocketDevices method.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice?language=objc
type VirtioSocketDevice struct {
	dispatchQueue unsafe.Pointer
	*pointer
}

func newVirtioSocketDevice(ptr, dispatchQueue unsafe.Pointer) *VirtioSocketDevice {
	return &VirtioSocketDevice{
		dispatchQueue: dispatchQueue,
		pointer:       objc.NewPointer(ptr),
	}
}

// socketListenerDelegateClass is the Go-defined Objective-C class that backs a
// VZVirtioSocketListener's delegate. Its single method forwards
// listener:shouldAcceptNewConnection:fromSocketDevice: to the Go handler
// associated with the delegate instance.
var (
	socketListenerDelegateClass objc.Class
	socketListenerDelegateOnce  sync.Once
)

func socketListenerDelegate() objc.Class {
	socketListenerDelegateOnce.Do(func() {
		cls, err := objc.DefineClass(
			"VZVirtioSocketListenerDelegateGo",
			objc.NSObjectClass(),
			[]objc.MethodDef{{
				Cmd: objc.RegisterName("listener:shouldAcceptNewConnection:fromSocketDevice:"),
				Fn:  socketListenerShouldAccept,
			}},
		)
		if err != nil {
			panic("vz: failed to define socket listener delegate: " + err.Error())
		}
		socketListenerDelegateClass = cls
	})
	return socketListenerDelegateClass
}

// socketListenerShouldAccept implements
// -[delegate listener:shouldAcceptNewConnection:fromSocketDevice:]. It always
// accepts the connection and hands it to the listener's Go handler.
func socketListenerShouldAccept(self objc.ID, _ objc.SEL, _, conn, _ unsafe.Pointer) bool {
	handler, ok := objc.Associated(uintptr(self)).(func(*VirtioSocketConnection, error))
	if !ok {
		return true
	}
	c, err := newVirtioSocketConnection(conn)
	go handler(c, err)
	return true
}

// Listen creates a new VirtioSocketListener which is a struct that listens for port-based connection requests
// from the guest operating system.
//
// Be sure to close the listener by calling `VirtioSocketListener.Close` after used this one.
//
// This is only supported on macOS 11 and newer, error will
// be returned on older versions.
func (v *VirtioSocketDevice) Listen(port uint32) (*VirtioSocketListener, error) {
	if err := macOSAvailable(11); err != nil {
		return nil, err
	}

	ch := make(chan connResults, 1) // should I increase more caps?
	handler := func(conn *VirtioSocketConnection, err error) {
		ch <- connResults{conn, err}
	}

	listenerPtr := objc.New("VZVirtioSocketListener", "init")
	delegate := objc.NewObject(socketListenerDelegate())
	objc.Associate(uintptr(delegate), handler)
	objc.SendVoid(listenerPtr, "setDelegate:", delegate)

	listener := &VirtioSocketListener{
		pointer:     objc.NewPointer(listenerPtr),
		vsockDevice: v,
		port:        port,
		delegate:    delegate,
		acceptch:    ch,
	}

	objc.DispatchSync(v.dispatchQueue, func() {
		objc.SendVoid(objc.Ptr(v), "setSocketListener:forPort:", objc.Ptr(listener), port)
	})

	return listener, nil
}

// Connect Initiates a connection to the specified port of the guest operating system.
//
// This method initiates the connection asynchronously, and executes the completion handler when the results are available.
// If the guest operating system doesn’t listen for connections to the specified port, this method does nothing.
//
// For a successful connection, this method sets the sourcePort property of the resulting VZVirtioSocketConnection object to a random port number.
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketdevice/3656677-connecttoport?language=objc
func (v *VirtioSocketDevice) Connect(port uint32) (*VirtioSocketConnection, error) {
	ch := make(chan connResults, 1)
	handler := func(conn *VirtioSocketConnection, err error) {
		ch <- connResults{conn, err}
		close(ch)
	}
	block := objc.BlockObjectError(func(connPtr, errPtr unsafe.Pointer) {
		if err := newNSError(errPtr); err != nil {
			handler(nil, err)
		} else {
			conn, err := newVirtioSocketConnection(connPtr)
			handler(conn, err)
		}
	})
	objc.DispatchAsync(v.dispatchQueue, func() {
		objc.SendVoid(objc.Ptr(v), "connectToPort:completionHandler:", port, block)
	})
	result := <-ch
	block.Release()
	runtime.KeepAlive(v)
	return result.conn, result.err
}

type connResults struct {
	conn *VirtioSocketConnection
	err  error
}

// VirtioSocketListener a struct that listens for port-based connection requests from the guest operating system.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketlistener?language=objc
type VirtioSocketListener struct {
	*pointer
	vsockDevice *VirtioSocketDevice
	delegate    unsafe.Pointer
	port        uint32
	acceptch    chan connResults
	closeOnce   sync.Once
}

var _ net.Listener = (*VirtioSocketListener)(nil)

// Accept implements the Accept method in the Listener interface; it waits for the next call and returns a net.Conn.
func (v *VirtioSocketListener) Accept() (net.Conn, error) {
	return v.AcceptVirtioSocketConnection()
}

// AcceptVirtioSocketConnection accepts the next incoming call and returns the new connection.
func (v *VirtioSocketListener) AcceptVirtioSocketConnection() (*VirtioSocketConnection, error) {
	result := <-v.acceptch
	return result.conn, result.err
}

// Close stops listening on the virtio socket.
func (v *VirtioSocketListener) Close() error {
	v.closeOnce.Do(func() {
		objc.DispatchSync(v.vsockDevice.dispatchQueue, func() {
			objc.SendVoid(objc.Ptr(v.vsockDevice), "removeSocketListenerForPort:", v.port)
		})
		objc.Disassociate(uintptr(v.delegate))
		objc.SendVoid(v.delegate, "release")
		v.acceptch <- connResults{
			conn: nil,
			err:  errors.New("accept failed: listener has been closed"),
		}
		close(v.acceptch)
	})
	return nil
}

// Addr returns the listener's network address, a *VirtioSocketListenerAddr.
func (v *VirtioSocketListener) Addr() net.Addr {
	const VMADDR_CID_HOST = 2 // copied from unix pacage
	return &VirtioSocketListenerAddr{
		CID:  VMADDR_CID_HOST,
		Port: v.port,
	}
}

// VirtioSocketListenerAddr represents a network end point address for the vsock protocol.
type VirtioSocketListenerAddr struct {
	CID  uint32
	Port uint32
}

var _ net.Addr = (*VirtioSocketListenerAddr)(nil)

// Network returns "vsock".
func (a *VirtioSocketListenerAddr) Network() string { return "vsock" }

// String returns string of "<cid>:<port>"
func (a *VirtioSocketListenerAddr) String() string { return fmt.Sprintf("%d:%d", a.CID, a.Port) }

// VirtioSocketConnection is a port-based connection between the guest operating system and the host computer.
//
// You don’t create connection objects directly. When the guest operating system initiates a connection, the virtual machine creates
// the connection object and passes it to the appropriate VirtioSocketListener struct, which forwards the object to its delegate.
//
// This is implemented net.Conn interface. This is generated from duplicated a file descriptor which is returned
// from virtualization.framework. macOS cannot connect directly to the Guest operating system using vsock. The　vsock
// connection must always be made via virtualization.framework. The diagram looks like this.
//
// ┌─────────┐                     ┌────────────────────────────┐               ┌────────────┐
// │  macOS  │<─── unix socket ───>│  virtualization.framework  │<─── vsock ───>│  Guest OS  │
// └─────────┘                     └────────────────────────────┘               └────────────┘
//
// You will notice that this is not vsock in using this library. However, all data this connection goes through to the vsock
// connection to which the Guest OS is connected.
//
// This struct does not have any pointers for objects of the Objective-C. Because the various values
// of the VZVirtioSocketConnection object handled by Objective-C are no longer needed after the conversion
// to the Go struct.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosocketconnection?language=objc
type VirtioSocketConnection struct {
	rawConn         net.Conn
	destinationPort uint32
	sourcePort      uint32
}

var _ net.Conn = (*VirtioSocketConnection)(nil)

func newVirtioSocketConnection(ptr unsafe.Pointer) (*VirtioSocketConnection, error) {
	id := objc.ID(uintptr(ptr))
	fd := objc.Send[int32](id, objc.RegisterName("fileDescriptor"))
	file := os.NewFile(uintptr(fd), "")
	defer file.Close()
	rawConn, err := net.FileConn(file)
	if err != nil {
		return nil, err
	}
	conn := &VirtioSocketConnection{
		rawConn:         rawConn,
		destinationPort: objc.Send[uint32](id, objc.RegisterName("destinationPort")),
		sourcePort:      objc.Send[uint32](id, objc.RegisterName("sourcePort")),
	}
	return conn, nil
}

// Read reads data from connection of the vsock protocol.
func (v *VirtioSocketConnection) Read(b []byte) (n int, err error) { return v.rawConn.Read(b) }

// Write writes data to the connection of the vsock protocol.
func (v *VirtioSocketConnection) Write(b []byte) (n int, err error) { return v.rawConn.Write(b) }

// Close will be called when caused something error in socket.
func (v *VirtioSocketConnection) Close() error {
	return v.rawConn.Close()
}

// LocalAddr returns the local network address.
func (v *VirtioSocketConnection) LocalAddr() net.Addr { return v.rawConn.LocalAddr() }

// RemoteAddr returns the remote network address.
func (v *VirtioSocketConnection) RemoteAddr() net.Addr { return v.rawConn.RemoteAddr() }

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
func (v *VirtioSocketConnection) SetDeadline(t time.Time) error { return v.rawConn.SetDeadline(t) }

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (v *VirtioSocketConnection) SetReadDeadline(t time.Time) error {
	return v.rawConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (v *VirtioSocketConnection) SetWriteDeadline(t time.Time) error {
	return v.rawConn.SetWriteDeadline(t)
}

// DestinationPort returns the destination port number of the connection.
func (v *VirtioSocketConnection) DestinationPort() uint32 {
	return v.destinationPort
}

// SourcePort returns the source port number of the connection.
func (v *VirtioSocketConnection) SourcePort() uint32 {
	return v.sourcePort
}
