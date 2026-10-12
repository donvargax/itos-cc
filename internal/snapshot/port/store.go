// Package port defines what the snapshot domain reaches outside itself: the
// project root its snapshots live under, and the snapshots themselves, read
// and written by name.
package port

import "context"

// Failure is why a snapshot could not be read or written, without exposing
// an operating system's or a decoder's own error types to the domain.
//
//sumtype:decl
type Failure interface {
	error
	snapshotFailure()
}

// Unreadable is returned when a snapshot that may be there could not be
// read, or what it holds could not be decoded. Name is the snapshot's, and
// Reason the library's error text whole, a sentence for people.
type Unreadable struct{ Name, Reason string }

func (*Unreadable) snapshotFailure() {}
func (e *Unreadable) Error() string  { return e.Reason }

// Unwritable is returned when a snapshot could not be encoded or stored.
// Name is the snapshot's, and Reason the library's error text whole.
type Unwritable struct{ Name, Reason string }

func (*Unwritable) snapshotFailure() {}
func (e *Unwritable) Error() string  { return e.Reason }

// Cancelled is returned when the context was done before the snapshot was
// read or written, as an interrupt makes it.
type Cancelled struct{}

func (*Cancelled) snapshotFailure() {}
func (*Cancelled) Error() string    { return "cancelled snapshot" }

// Store is where a project's snapshots live: .metrics/ at its root. ctx
// carries only cancellation and deadlines, never a value (decision 30);
// once it is done, Root, Read and Write are Cancelled and nothing is
// written. porttest.StoreContract holds what every Store, the fake and the
// disk's, must do.
type Store interface {
	// Root is the project root, absolute: where the snapshots live, and
	// what every path a snapshot records is relative to (snapshot.Key).
	Root(ctx context.Context) (string, Failure)
	// Read decodes the snapshot name into v and says whether it was there.
	// One that is not there is false and no failure, v untouched. One that
	// could not be read, or decoded, is Unreadable; true when it was there,
	// v then holding what the decoding filled before it stopped.
	Read(ctx context.Context, name string, v any) (bool, Failure)
	// Write stores v as the snapshot name, replacing it whole, so a reader
	// never sees half a snapshot.
	Write(ctx context.Context, name string, v any) Failure
}
