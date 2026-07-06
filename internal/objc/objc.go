package objc

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	pobjc "github.com/ebitengine/purego/objc"
)

// foundationPath is the Foundation framework loaded so its Objective-C classes
// (NSMutableArray, NSMutableDictionary, NSString, NSUUID) resolve via the
// Objective-C runtime. When the cgo-based parent package is also linked, the
// framework is already loaded and this dlopen simply returns the existing
// handle.
const foundationPath = "/System/Library/Frameworks/Foundation.framework/Foundation"

// Cached classes and selectors. Resolving a selector grabs the global
// Objective-C lock, so they are looked up once at init time and reused.
var (
	classNSMutableArray      pobjc.Class
	classNSMutableDictionary pobjc.Class
	classNSString            pobjc.Class
	classNSUUID              pobjc.Class
	classNSData              pobjc.Class

	selAlloc                pobjc.SEL
	selInit                 pobjc.SEL
	selInitWithCapacity     pobjc.SEL
	selAddObject            pobjc.SEL
	selSetValueForKey       pobjc.SEL
	selRelease              pobjc.SEL
	selRetain               pobjc.SEL
	selCount                pobjc.SEL
	selObjectAtIndex        pobjc.SEL
	selStringWithUTF8String pobjc.SEL
	selUUID                 pobjc.SEL
	selUUIDString           pobjc.SEL
	selUTF8String           pobjc.SEL
	selInitWithBytesLength  pobjc.SEL
	selBytes                pobjc.SEL
	selLength               pobjc.SEL
	selAutorelease          pobjc.SEL

	// dispatchRelease is bound to libdispatch's dispatch_release when present.
	dispatchRelease func(unsafe.Pointer)
)

func init() {
	if _, err := purego.Dlopen(foundationPath, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		panic("objc: failed to load Foundation: " + err.Error())
	}

	classNSMutableArray = pobjc.GetClass("NSMutableArray")
	classNSMutableDictionary = pobjc.GetClass("NSMutableDictionary")
	classNSString = pobjc.GetClass("NSString")
	classNSUUID = pobjc.GetClass("NSUUID")
	classNSData = pobjc.GetClass("NSData")

	selAlloc = pobjc.RegisterName("alloc")
	selInit = pobjc.RegisterName("init")
	selInitWithCapacity = pobjc.RegisterName("initWithCapacity:")
	selAddObject = pobjc.RegisterName("addObject:")
	selSetValueForKey = pobjc.RegisterName("setValue:forKey:")
	selRelease = pobjc.RegisterName("release")
	selRetain = pobjc.RegisterName("retain")
	selCount = pobjc.RegisterName("count")
	selObjectAtIndex = pobjc.RegisterName("objectAtIndex:")
	selStringWithUTF8String = pobjc.RegisterName("stringWithUTF8String:")
	selUUID = pobjc.RegisterName("UUID")
	selUUIDString = pobjc.RegisterName("UUIDString")
	selUTF8String = pobjc.RegisterName("UTF8String")
	selInitWithBytesLength = pobjc.RegisterName("initWithBytes:length:")
	selBytes = pobjc.RegisterName("bytes")
	selLength = pobjc.RegisterName("length")
	selAutorelease = pobjc.RegisterName("autorelease")

	if sym, err := purego.Dlsym(purego.RTLD_DEFAULT, "dispatch_release"); err == nil && sym != 0 {
		purego.RegisterFunc(&dispatchRelease, sym)
	}
}

// objcID converts an Objective-C object pointer into a purego message receiver.
// The conversion direction (pointer to uintptr) is safe for go vet because the
// pointer addresses an Objective-C heap object that the Go GC never moves.
func objcID(p unsafe.Pointer) pobjc.ID {
	return pobjc.ID(uintptr(p))
}

// ReleaseDispatch releases allocated dispatch_queue_t.
func ReleaseDispatch(p unsafe.Pointer) {
	if dispatchRelease != nil {
		dispatchRelease(p)
		return
	}
	objcID(p).Send(selRelease)
}

// Pointer indicates any pointers which are allocated in objective-c world.
type Pointer struct {
	_ptr unsafe.Pointer
}

// NewPointer creates a new Pointer for objc.
func NewPointer(p unsafe.Pointer) *Pointer {
	return &Pointer{_ptr: p}
}

// release releases allocated resources in objective-c world.
// decrements reference count.
func (p *Pointer) release() {
	objcID(p._ptr).Send(selRelease)
	runtime.KeepAlive(p)
}

// retain increments reference count in objective-c world.
func (p *Pointer) retain() {
	objcID(p._ptr).Send(selRetain)
	runtime.KeepAlive(p)
}

// ptr returns raw pointer.
func (o *Pointer) ptr() unsafe.Pointer {
	if o == nil {
		return nil
	}
	return o._ptr
}

// NSObject indicates NSObject.
type NSObject interface {
	ptr() unsafe.Pointer
	release()
	retain()
}

// Release releases allocated resources in objective-c world.
func Release(o NSObject) {
	o.release()
}

// Retain increments reference count in objective-c world.
func Retain(o NSObject) {
	o.retain()
}

// Ptr returns unsafe.Pointer of the NSObject.
func Ptr(o NSObject) unsafe.Pointer {
	return o.ptr()
}

// NSArray indicates NSArray.
type NSArray struct {
	*Pointer
}

// NewNSArray creates a new NSArray from pointer.
func NewNSArray(p unsafe.Pointer) *NSArray {
	return &NSArray{NewPointer(p)}
}

// ToPointerSlice method returns slice of the obj-c object as unsafe.Pointer.
func (n *NSArray) ToPointerSlice() []unsafe.Pointer {
	id := objcID(n.ptr())
	count := int(pobjc.Send[uint64](id, selCount))
	ret := make([]unsafe.Pointer, count)
	for i := range count {
		ret[i] = pobjc.Send[unsafe.Pointer](id, selObjectAtIndex, uint64(i))
	}
	runtime.KeepAlive(n)
	return ret
}

// ConvertToNSMutableArray converts to NSMutableArray from NSObject slice in Go world.
func ConvertToNSMutableArray(s []NSObject) *Pointer {
	alloc := pobjc.ID(classNSMutableArray).Send(selAlloc)
	arrayPtr := pobjc.Send[unsafe.Pointer](alloc, selInitWithCapacity, uint64(len(s)))
	id := objcID(arrayPtr)
	for _, v := range s {
		id.Send(selAddObject, v.ptr())
	}
	p := NewPointer(arrayPtr)
	runtime.SetFinalizer(p, func(self *Pointer) {
		self.release()
	})
	return p
}

// ConvertToNSMutableDictionary converts to NSMutableDictionary from map[string]NSObject in Go world.
func ConvertToNSMutableDictionary(d map[string]NSObject) *Pointer {
	alloc := pobjc.ID(classNSMutableDictionary).Send(selAlloc)
	dictPtr := pobjc.Send[unsafe.Pointer](alloc, selInit)
	id := objcID(dictPtr)
	for key, value := range d {
		nskey := pobjc.ID(classNSString).Send(selStringWithUTF8String, append([]byte(key), 0))
		id.Send(selSetValueForKey, value.ptr(), nskey)
	}
	p := NewPointer(dictPtr)
	runtime.SetFinalizer(p, func(self *Pointer) {
		self.release()
	})
	return p
}

// GetUUID returns a pointer to a NUL-terminated C string holding a new UUID.
// The string is owned by the Objective-C autorelease machinery and must be used
// immediately; callers must not free it.
func GetUUID() unsafe.Pointer {
	uuid := pobjc.ID(classNSUUID).Send(selUUID)
	str := uuid.Send(selUUIDString)
	return pobjc.Send[unsafe.Pointer](str, selUTF8String)
}
