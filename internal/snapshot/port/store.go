// Package port defines what the snapshot domain reaches outside itself: the
// project root its snapshots live under, and the snapshots themselves, read,
// written and removed by name, and listed by folder.
package port

import "context"

// Failure is why a snapshot could not be read, written, removed or listed,
// without exposing an operating system's or a decoder's own error types to
// the domain.
//
//sumtype:decl
type Failure interface {
	error
	snapshotFailure()
}

// Unreadable is returned when a snapshot that may be there could not be
// read, or what it holds could not be decoded. Name is the snapshot's, and
// Reason a sentence for people: the library's error text whole for a
// snapshot that could not be read, and for one that did not decode,
// Undecodable's, in the snapshot's own terms.
type Unreadable struct{ Name, Reason string }

func (*Unreadable) snapshotFailure() {}
func (e *Unreadable) Error() string  { return e.Reason }

// Unwritable is returned when a snapshot could not be encoded or stored.
// Name is the snapshot's, and Reason the library's error text whole.
type Unwritable struct{ Name, Reason string }

func (*Unwritable) snapshotFailure() {}
func (e *Unwritable) Error() string  { return e.Reason }

// Unremovable is returned when a snapshot that may be there could not be
// removed. Name is the snapshot's, and Reason the library's error text
// whole.
type Unremovable struct{ Name, Reason string }

func (*Unremovable) snapshotFailure() {}
func (e *Unremovable) Error() string  { return e.Reason }

// Unlistable is returned when a folder of snapshots could not be read.
// Dir is the folder's name, as List was given it, and Reason the library's
// error text whole.
type Unlistable struct{ Dir, Reason string }

func (*Unlistable) snapshotFailure() {}
func (e *Unlistable) Error() string  { return e.Reason }

// Cancelled is returned when the context was done before the snapshot was
// read, written or removed, or its folder listed, as an interrupt makes it.
type Cancelled struct{}

func (*Cancelled) snapshotFailure() {}
func (*Cancelled) Error() string    { return "cancelled snapshot" }

// Store is where a project's snapshots live: .metrics/ at its root. A
// snapshot's name is its path from there, such as dry.json or
// mutate/src/a.go.json. ctx carries only cancellation and deadlines, never
// a value (decision 30); once it is done, every method is Cancelled and
// nothing is written or removed. porttest.StoreContract holds what every
// Store, the fake and the disk's, must do.
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
	// Remove deletes the snapshot name, then each folder that deleting it
	// left empty, up to the folder at the top of name, such as mutate/,
	// which stays, as .metrics/ does. One that is not there is no failure;
	// one that could not be deleted is Unremovable. A folder that holds
	// anything else, or cannot be deleted, is left as it is.
	Remove(ctx context.Context, name string) Failure
	// List is the names of the snapshots under the folder dir of .metrics/,
	// at any depth, as Read takes them: from .metrics/, slash-separated and
	// sorted. Only a regular file whose name ends in .json is a snapshot; a
	// staging file of a write, a folder, a link or anything else is none.
	// A folder that is not there holds none; one that could not be read is
	// Unlistable.
	List(ctx context.Context, dir string) ([]string, Failure)
}
