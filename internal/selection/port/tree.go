// Package port defines what the selection domain reaches outside itself:
// the files under some roots, the ones git reports as changed, whether a
// path is there, and what a selected file holds.
package port

import "context"

// Failure is why a selection failed, without exposing an operating system's
// or git's own error types to the domain.
//
//sumtype:decl
type Failure interface {
	error
	selectionFailure()
}

// NoGit is returned when the changed files were asked for and there is no
// git repository to ask, or no git. Reason is what git said, or why it
// could not be run.
type NoGit struct{ Reason string }

func (*NoGit) selectionFailure() {}
func (e *NoGit) Error() string   { return "no git repository: " + e.Reason }

// Unmatched is returned when an argument names no path and no selectable
// file's path contains it: Fragment is the argument, as the domain matched
// it.
type Unmatched struct{ Fragment string }

func (*Unmatched) selectionFailure() {}
func (e *Unmatched) Error() string   { return "no file matches " + e.Fragment }

// Unwalkable is returned when a root, or a directory under it, could not be
// walked. Reason is the walk's own message, a sentence for people.
type Unwalkable struct{ Reason string }

func (*Unwalkable) selectionFailure() {}
func (e *Unwalkable) Error() string   { return e.Reason }

// Unreadable is returned when a selected file could not be read. Reason is
// the read's own message, a sentence for people.
type Unreadable struct{ Path, Reason string }

func (*Unreadable) selectionFailure() {}
func (e *Unreadable) Error() string   { return e.Reason }

// Cancelled is returned when the context was done before the files were
// found, as an interrupt makes it.
type Cancelled struct{}

func (*Cancelled) selectionFailure() {}
func (*Cancelled) Error() string     { return "cancelled selection" }

// Tree is the working tree a command selects its files from. ctx carries
// only cancellation and deadlines, never a value.
type Tree interface {
	// Discover is every supported file under roots, files or directories,
	// split into production sources and tests, each list sorted; a root
	// naming a file is taken even where a walk would skip it.
	Discover(ctx context.Context, roots []string) (sources, tests []string, failure Failure)
	// Changed is the supported files git reports as added, modified or
	// untracked under the working directory, relative to it.
	Changed(ctx context.Context) ([]string, Failure)
	// Exists says whether path names a file or a directory.
	Exists(ctx context.Context, path string) bool
	// Read is what the file at path, as Discover named it, holds. Every
	// command that selects files reads them, so it reads them here, through
	// the tree it chose them from, never by itself (decision 29).
	Read(ctx context.Context, path string) ([]byte, Failure)
}
