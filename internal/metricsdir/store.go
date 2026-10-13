// Package metricsdir adapts the snapshots under .metrics/ at a project's
// root, by default that of this process's working directory, to the
// snapshot domain's Store port: it wraps project, which finds the root, and
// metrics, which reads snapshots, writes them as indented JSON renamed into
// place, removes them and lists them, so every byte and every atomic write
// stays the library's, and translates what fails into the port's failures.
package metricsdir

import (
	"context"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// Store is the snapshots of the project whose root is RootDir, or, its zero
// value, of the project this process runs in, found again at each call.
// One Store for each root serves several projects with no change to the
// port.
type Store struct {
	// RootDir is the project root, absolute; "" is project.Root, the git
	// top level of the working directory or the working directory itself.
	RootDir string
}

var _ port.Store = Store{}

// Root is RootDir, or project.Root when it is "". A done ctx asks nothing.
func (s Store) Root(ctx context.Context) (string, port.Failure) {
	if ctx.Err() != nil {
		return "", &port.Cancelled{}
	}
	return s.root(), nil
}

// Read is metrics.ReadIn, any failure Unreadable with the library's text
// whole, as the command line has always printed it. A done ctx reads
// nothing; a read under way runs to its end.
func (s Store) Read(ctx context.Context, name string, v any) (bool, port.Failure) {
	if ctx.Err() != nil {
		return false, &port.Cancelled{}
	}
	found, err := metrics.ReadIn(s.dir(), name, v)
	if err != nil {
		return found, &port.Unreadable{Name: name, Reason: err.Error()}
	}
	return found, nil
}

// Write is metrics.WriteIn, any failure Unwritable with the library's text
// whole. A done ctx writes nothing; a write under way runs to its end, so
// no snapshot is left half renamed.
func (s Store) Write(ctx context.Context, name string, v any) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{}
	}
	if err := metrics.WriteIn(s.dir(), name, v); err != nil {
		return &port.Unwritable{Name: name, Reason: err.Error()}
	}
	return nil
}

// Remove is metrics.RemoveIn, any failure Unremovable with the library's
// text whole. A done ctx removes nothing; a removal under way runs to its
// end.
func (s Store) Remove(ctx context.Context, name string) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{}
	}
	if err := metrics.RemoveIn(s.dir(), name); err != nil {
		return &port.Unremovable{Name: name, Reason: err.Error()}
	}
	return nil
}

// List is metrics.ListIn, any failure Unlistable with the library's text
// whole. A done ctx lists nothing; a listing under way runs to its end.
func (s Store) List(ctx context.Context, dir string) ([]string, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	names, err := metrics.ListIn(s.dir(), dir)
	if err != nil {
		return nil, &port.Unlistable{Dir: dir, Reason: err.Error()}
	}
	return names, nil
}

// root is the project root the store's snapshots live under.
func (s Store) root() string {
	if s.RootDir != "" {
		return s.RootDir
	}
	return project.Root()
}

// dir is .metrics at the root.
func (s Store) dir() string {
	return metrics.DirOf(s.root())
}
