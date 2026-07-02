package vz

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// newCustomVirtioDevice wraps the framework device delivered to didCreateDevice: and
// takes over ownership of the +1 Go-backed delegate and +1 dispatch queue from the
// configuration (the plan step-8 transfer): the wrapper keeps them alive for the
// virtual machine's run and releases everything in its finalizer, and the
// configuration's transferred flag is set so its own finalizer stops owning them. It
// runs on the device queue (inside the didCreateDevice: IMP).
func newCustomVirtioDevice(device, delegate, queue unsafe.Pointer, config *CustomVirtioDeviceConfiguration) *CustomVirtioDevice {
	objc.SendVoid(device, "retain") // +1 the framework device for the wrapper's lifetime
	d := &CustomVirtioDevice{
		pointer:  objc.NewPointer(device),
		delegate: delegate,
		queue:    queue,
	}
	config.transferred.Store(true)
	objc.SetFinalizer(d, func(self *CustomVirtioDevice) {
		objc.Disassociate(uintptr(self.delegate))
		objc.SendVoid(self.delegate, "release")
		objc.ReleaseDispatch(self.queue)
		objc.Release(self) // release the +1 framework device
	})
	return d
}

// QueueAtIndex returns the virtqueue at index, or nil if the index is invalid or the
// guest driver has disabled the queue. The result is only valid after the guest driver
// reaches DRIVER_OK (CustomVirtioHandler.DidAcceptDriverOK); call it on the device
// queue (the context of the handler callbacks).
func (d *CustomVirtioDevice) QueueAtIndex(index uint16) *VirtioQueue {
	ptr := objc.Send[unsafe.Pointer](objc.ID(uintptr(objc.Ptr(d))), objc.RegisterName("queueAtIndex:"), index)
	if ptr == nil {
		return nil
	}
	return newVirtioQueue(ptr)
}

// GuestMemoryMappingAtPhysicalAddress returns a mapping of guest DRAM at the given
// guest physical address and length, or nil if the range does not reference valid
// guest RAM. The mapping is invalidated across a guest reboot or shutdown.
func (d *CustomVirtioDevice) GuestMemoryMappingAtPhysicalAddress(physicalAddress uint64, length uintptr) *GuestMemoryMapping {
	// The Objective-C length argument is size_t (64-bit on arm64); uintptr marshals it.
	ptr := objc.Send[unsafe.Pointer](
		objc.ID(uintptr(objc.Ptr(d))),
		objc.RegisterName("guestMemoryMappingAtPhysicalAddress:length:"),
		physicalAddress, length,
	)
	if ptr == nil {
		return nil
	}
	return newGuestMemoryMapping(ptr)
}

// NegotiatedFeatures returns the set of Virtio features the driver and device
// negotiated. The second result is false before the guest driver reaches DRIVER_OK.
func (d *CustomVirtioDevice) NegotiatedFeatures() (*NegotiatedVirtioFeatureSet, bool) {
	ptr := objc.SendPtr(objc.Ptr(d), "negotiatedFeatures")
	if ptr == nil {
		return nil, false
	}
	return newNegotiatedVirtioFeatureSet(ptr), true
}

// RequestDeviceReset asks the guest to reset the device (it sets DEVICE_NEEDS_RESET).
// The guest may or may not act; the framework calls CustomVirtioHandler.WillReset when
// the reset completes.
func (d *CustomVirtioDevice) RequestDeviceReset() {
	objc.SendVoid(objc.Ptr(d), "requestDeviceReset")
}

// newVirtioQueue wraps a framework-owned VZVirtioQueue (a +0 property of the device),
// retaining it so the wrapper can outlive a single callback and releasing it in the
// finalizer.
func newVirtioQueue(ptr unsafe.Pointer) *VirtioQueue {
	objc.SendVoid(ptr, "retain")
	q := &VirtioQueue{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(q, func(self *VirtioQueue) {
		objc.Release(self)
	})
	return q
}

// NextElement returns the next available element (descriptor chain) on the queue, or
// nil when none remain. Process elements in a loop until it returns nil — the framework
// disables virtqueue notifications until the queue is fully drained. Call it on the
// device queue.
func (q *VirtioQueue) NextElement() *VirtioQueueElement {
	ptr := objc.SendPtr(objc.Ptr(q), "nextElement")
	if ptr == nil {
		return nil
	}
	return &VirtioQueueElement{pointer: objc.NewPointer(ptr)}
}

// QueueIndex returns the index of this queue.
func (q *VirtioQueue) QueueIndex() uint16 {
	return objc.Send[uint16](objc.ID(uintptr(objc.Ptr(q))), objc.RegisterName("queueIndex"))
}

// QueueSize returns the maximum number of elements the queue can present.
func (q *VirtioQueue) QueueSize() uint16 {
	return objc.Send[uint16](objc.ID(uintptr(objc.Ptr(q))), objc.RegisterName("queueSize"))
}

// VirtioQueueElement is a unit of work (descriptor chain) from a VirtioQueue, exposing
// the guest's scatter-gather read buffers (guest → host) and write buffers
// (host → guest). Obtain one from VirtioQueue.NextElement and call ReturnToQueue exactly
// once when done. It is only valid between NextElement and ReturnToQueue, on the device
// queue; do not copy it or hold it beyond that.
//
// Important: accessing the underlying guest memory more than once can introduce
// time-of-check/time-of-use (TOCTOU) bugs, since the guest may modify its memory at any
// time. Prefer the copy-by-default ReadBytes; treat ReadBuffers/ReadInto/WriteBuffer as
// advanced zero-copy paths.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtioqueueelement?language=objc
type VirtioQueueElement struct {
	*pointer
	returned bool
}

func (e *VirtioQueueElement) uintProp(sel string) uint64 {
	return objc.Send[uint64](objc.ID(uintptr(objc.Ptr(e))), objc.RegisterName(sel))
}

// ReadBuffersByteCount returns the total size of the read buffers, in bytes.
func (e *VirtioQueueElement) ReadBuffersByteCount() uint64 {
	return e.uintProp("readBuffersByteCount")
}

// ReadBuffersAvailableByteCount returns the read-buffer bytes not yet consumed.
func (e *VirtioQueueElement) ReadBuffersAvailableByteCount() uint64 {
	return e.uintProp("readBuffersAvailableByteCount")
}

// WriteBuffersByteCount returns the total size of the write buffers, in bytes.
func (e *VirtioQueueElement) WriteBuffersByteCount() uint64 {
	return e.uintProp("writeBuffersByteCount")
}

// WriteBuffersAvailableByteCount returns the write-buffer bytes not yet written.
func (e *VirtioQueueElement) WriteBuffersAvailableByteCount() uint64 {
	return e.uintProp("writeBuffersAvailableByteCount")
}

// WrittenByteCount returns the number of write-buffer bytes written so far.
func (e *VirtioQueueElement) WrittenByteCount() uint64 {
	return e.uintProp("writtenByteCount")
}

// ReadBytes reads exactly length bytes from the read buffers into a new slice,
// consuming them.
func (e *VirtioQueueElement) ReadBytes(length int) ([]byte, error) {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	data := objc.Send[unsafe.Pointer](
		objc.ID(uintptr(objc.Ptr(e))),
		objc.RegisterName("readBytesWithExactLength:error:"),
		uint(length), errSlot,
	)
	if objc.HasError(errSlot) {
		return nil, newNSError(objc.ErrorFromSlot(errSlot))
	}
	return objc.NSDataToBytes(data), nil
}

// Peek copies exactly length bytes from the read buffers WITHOUT consuming them (the
// available byte count is unchanged).
func (e *VirtioQueueElement) Peek(length int) ([]byte, error) {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	data := objc.Send[unsafe.Pointer](
		objc.ID(uintptr(objc.Ptr(e))),
		objc.RegisterName("peekIntoReadBuffersWithExactLength:error:"),
		uint(length), errSlot,
	)
	if objc.HasError(errSlot) {
		return nil, newNSError(objc.ErrorFromSlot(errSlot))
	}
	return objc.NSDataToBytes(data), nil
}

// WriteData writes data into the write buffers (host → guest). Complete all writes
// before calling ReturnToQueue.
func (e *VirtioQueueElement) WriteData(data []byte) error {
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	// NSData is +1; writeData:error: copies it synchronously, so release after.
	nsData := objc.NSData(data)
	objc.Send[bool](
		objc.ID(uintptr(objc.Ptr(e))),
		objc.RegisterName("writeData:error:"),
		nsData, errSlot,
	)
	objc.SendVoid(nsData, "release")
	if objc.HasError(errSlot) {
		return newNSError(objc.ErrorFromSlot(errSlot))
	}
	return nil
}

// ReadBuffers returns the remaining read buffers as byte slices, consuming them all;
// each slice is a copy of a guest scatter-gather segment. Advanced — see the TOCTOU
// note on VirtioQueueElement.
func (e *VirtioQueueElement) ReadBuffers() [][]byte {
	arr := objc.NewNSArray(objc.SendPtr(objc.Ptr(e), "readBuffers"))
	ptrs := arr.ToPointerSlice()
	out := make([][]byte, len(ptrs))
	for i, p := range ptrs {
		out[i] = objc.NSDataToBytes(p)
	}
	return out
}

// ReadInto reads exactly len(buf) bytes from the read buffers directly into buf,
// consuming them. Advanced zero-copy path — see the TOCTOU note on VirtioQueueElement.
func (e *VirtioQueueElement) ReadInto(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	objc.Send[bool](
		objc.ID(uintptr(objc.Ptr(e))),
		objc.RegisterName("readBytesIntoBuffer:exactLength:error:"),
		unsafe.Pointer(&buf[0]), uint(len(buf)), errSlot,
	)
	runtime.KeepAlive(buf)
	if objc.HasError(errSlot) {
		return newNSError(objc.ErrorFromSlot(errSlot))
	}
	return nil
}

// WriteBuffer writes exactly len(buf) bytes from buf into the write buffers
// (host → guest). Advanced zero-copy path. Complete all writes before ReturnToQueue.
func (e *VirtioQueueElement) WriteBuffer(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	errSlot := objc.NewErrorSlot()
	defer objc.Free(errSlot)
	objc.Send[bool](
		objc.ID(uintptr(objc.Ptr(e))),
		objc.RegisterName("writeBuffer:exactLength:error:"),
		unsafe.Pointer(&buf[0]), uint(len(buf)), errSlot,
	)
	runtime.KeepAlive(buf)
	if objc.HasError(errSlot) {
		return newNSError(objc.ErrorFromSlot(errSlot))
	}
	return nil
}

// ReturnToQueue returns this element to the guest. Call it exactly once, after
// processing. A second call is ignored: a real Objective-C double-return raises an
// exception, which cannot be recovered across the purego boundary and would abort the
// process, so this guard is best-effort and per-wrapper — do not copy the element or
// share it across goroutines.
func (e *VirtioQueueElement) ReturnToQueue() error {
	if e.returned {
		return fmt.Errorf("vz: VirtioQueueElement already returned to the queue")
	}
	e.returned = true
	objc.SendVoid(objc.Ptr(e), "returnToQueue")
	return nil
}

// GuestMemoryMapping is a chunk of the guest operating system's DRAM, obtained from
// CustomVirtioDevice.GuestMemoryMappingAtPhysicalAddress. It provides direct read/write
// access to guest memory. It is invalidated across a guest reboot or shutdown and must
// not be used after; treat the pointer from MutableBytes as advanced/unsafe.
//
// see: https://developer.apple.com/documentation/virtualization/vzguestmemorymapping?language=objc
type GuestMemoryMapping struct {
	*pointer
}

func newGuestMemoryMapping(ptr unsafe.Pointer) *GuestMemoryMapping {
	objc.SendVoid(ptr, "retain")
	m := &GuestMemoryMapping{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(m, func(self *GuestMemoryMapping) {
		objc.Release(self)
	})
	return m
}

// MutableBytes returns a host pointer to the guest DRAM this mapping covers (Length
// bytes). It becomes invalid across a guest reboot or shutdown.
func (m *GuestMemoryMapping) MutableBytes() unsafe.Pointer {
	return objc.Send[unsafe.Pointer](objc.ID(uintptr(objc.Ptr(m))), objc.RegisterName("mutableBytes"))
}

// PhysicalAddress returns the guest physical base address of this mapping.
func (m *GuestMemoryMapping) PhysicalAddress() uint64 {
	return objc.Send[uint64](objc.ID(uintptr(objc.Ptr(m))), objc.RegisterName("physicalAddress"))
}

// Length returns the number of bytes this mapping covers.
func (m *GuestMemoryMapping) Length() uintptr {
	return uintptr(objc.Send[uint64](objc.ID(uintptr(objc.Ptr(m))), objc.RegisterName("length")))
}

// NegotiatedVirtioFeatureSet is the set of Virtio feature bits the device and driver
// negotiated, encoded as two 32-bit subsets. Obtain it from
// CustomVirtioDevice.NegotiatedFeatures after DRIVER_OK.
//
// see: https://developer.apple.com/documentation/virtualization/vznegotiatedvirtiofeatureset?language=objc
type NegotiatedVirtioFeatureSet struct {
	*pointer
}

func newNegotiatedVirtioFeatureSet(ptr unsafe.Pointer) *NegotiatedVirtioFeatureSet {
	objc.SendVoid(ptr, "retain")
	fs := &NegotiatedVirtioFeatureSet{pointer: objc.NewPointer(ptr)}
	objc.SetFinalizer(fs, func(self *NegotiatedVirtioFeatureSet) {
		objc.Release(self)
	})
	return fs
}

// Subset0 returns the negotiated feature bits 0 through 31.
func (f *NegotiatedVirtioFeatureSet) Subset0() uint32 {
	return objc.Send[uint32](objc.ID(uintptr(objc.Ptr(f))), objc.RegisterName("subset0"))
}

// Subset1 returns the negotiated feature bits 32 through 63.
func (f *NegotiatedVirtioFeatureSet) Subset1() uint32 {
	return objc.Send[uint32](objc.ID(uintptr(objc.Ptr(f))), objc.RegisterName("subset1"))
}
