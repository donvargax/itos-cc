// Package porttest provides in-memory fakes for counting-domain tests.
package porttest

import (
	"context"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Files holds test inputs and configured read failures in memory.
type Files struct {
	Data     map[string][]byte
	Failures map[string]port.ReadFailure
}

var _ port.Files = Files{}

// Read returns a copy of the configured file, or its configured failure. It
// does not act on ctx, since nothing here waits: a test that wants a
// cancelled read configures port.Cancelled as the path's failure.
func (f Files) Read(ctx context.Context, path string) ([]byte, port.ReadFailure) {
	if failure, ok := f.Failures[path]; ok {
		return nil, failure
	}
	data, ok := f.Data[path]
	if !ok {
		return nil, &port.Missing{Path: path}
	}
	return append([]byte(nil), data...), nil
}

// Unreadable returns a configured unreadable-file failure.
func Unreadable(path string) port.ReadFailure { return &port.Unreadable{Path: path} }
