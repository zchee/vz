package vz

import (
	"fmt"
	"unsafe"

	"github.com/Code-Hex/vz/v3/internal/objc"
)

// pointer is a type alias which is able to use as embedded type and
// makes as unexported it.
type pointer = objc.Pointer

// NSError indicates NSError.
type NSError struct {
	Domain               string
	Code                 int
	LocalizedDescription string
	UserInfo             string
}

func (n *NSError) Error() string {
	if n == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"Error Domain=%s Code=%d Description=%q UserInfo=%s",
		n.Domain,
		n.Code,
		n.LocalizedDescription,
		n.UserInfo,
	)
}

// newNSError converts an Objective-C NSError pointer into a Go *NSError. It
// returns nil when p is nil, which is the framework's convention for "no error".
func newNSError(p unsafe.Pointer) *NSError {
	if p == nil {
		return nil
	}
	return &NSError{
		Domain:               objc.ErrorDomain(p),
		Code:                 objc.ErrorCode(p),
		LocalizedDescription: objc.ErrorLocalizedDescription(p),
		UserInfo:             objc.ErrorUserInfo(p), // NOTE(codehex): maybe we can convert to map[string]interface{}
	}
}
