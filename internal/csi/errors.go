package csi

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func errInvalid(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}

func errNotFound(format string, args ...any) error {
	return status.Errorf(codes.NotFound, format, args...)
}

func errAlreadyExists(format string, args ...any) error {
	return status.Errorf(codes.AlreadyExists, format, args...)
}

func errInternal(format string, args ...any) error {
	return status.Errorf(codes.Internal, format, args...)
}

func errUnimplemented(format string, args ...any) error {
	return status.Errorf(codes.Unimplemented, format, args...)
}
