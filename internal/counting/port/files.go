// Package port defines the files the counting domain can read.
package port

import "context"

// ReadFailure describes a file-read failure without exposing operating-system
// error types to the domain.
//
//sumtype:decl
type ReadFailure interface {
	error
	readFailure()
}

// Missing is returned when the requested input does not exist.
type Missing struct{ Path string }

func (*Missing) readFailure()    {}
func (e *Missing) Error() string { return "missing file: " + e.Path }

// Unreadable is returned when the requested input exists but cannot be read.
type Unreadable struct{ Path string }

func (*Unreadable) readFailure()    {}
func (e *Unreadable) Error() string { return "unreadable file: " + e.Path }

// Cancelled is returned when the read's context was done before the read
// ended, as an interrupt makes it: the input was not read whole.
type Cancelled struct{ Path string }

func (*Cancelled) readFailure()    {}
func (e *Cancelled) Error() string { return "cancelled read: " + e.Path }

// Files reads the bytes at a path. Implementations return a typed ReadFailure
// for missing and unreadable inputs, and Cancelled for a read that waits and
// whose ctx is done first. ctx carries only the read's cancellation and
// deadline, never a value. A relative path is from the working directory.
// porttest.FilesContract holds what every Files, the fake and the disk's,
// must do.
type Files interface {
	Read(ctx context.Context, path string) ([]byte, ReadFailure)
}
