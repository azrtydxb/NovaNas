package runtime

import "errors"

var (
	ErrNotImplemented = errors.New("runtime: not implemented")
	ErrNotFound       = errors.New("runtime: not found")
	ErrAlreadyExists  = errors.New("runtime: already exists")
	ErrInvalidSpec    = errors.New("runtime: invalid spec")
)
