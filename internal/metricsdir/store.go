// Package metricsdir adapts the snapshots under .metrics/ at the project
// root of this process's working directory to the snapshot domain's Store
// port: it wraps project, which finds the root, and metrics, which reads
// snapshots and writes them as indented JSON renamed into place, so every
// byte and every atomic write stays the library's, and translates what
// fails into the port's failures.
package metricsdir

import (
	"context"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// Store is the snapshots of the project this process runs in.
type Store struct{}

var _ port.Store = Store{}

// Root is project.Root, the git top level of the working directory or the
// working directory itself. A done ctx asks nothing.
func (Store) Root(ctx context.Context) (string, port.Failure) {
	if ctx.Err() != nil {
		return "", &port.Cancelled{}
	}
	return project.Root(), nil
}

// Read is metrics.Read, any failure Unreadable with the library's text
// whole, as the command line has always printed it. A done ctx reads
// nothing; a read under way runs to its end.
func (Store) Read(ctx context.Context, name string, v any) (bool, port.Failure) {
	if ctx.Err() != nil {
		return false, &port.Cancelled{}
	}
	found, err := metrics.Read(name, v)
	if err != nil {
		return found, &port.Unreadable{Name: name, Reason: err.Error()}
	}
	return found, nil
}

// Write is metrics.Write, any failure Unwritable with the library's text
// whole. A done ctx writes nothing; a write under way runs to its end, so
// no snapshot is left half renamed.
func (Store) Write(ctx context.Context, name string, v any) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{}
	}
	if err := metrics.Write(name, v); err != nil {
		return &port.Unwritable{Name: name, Reason: err.Error()}
	}
	return nil
}
