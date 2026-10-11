// Package disk adapts read-only operating-system file access to the counting
// domain's Files port.
package disk

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Files reads input files from the local filesystem.
type Files struct{}

var _ port.Files = Files{}

// Read reads path without modifying or executing its contents and translates
// operating-system errors into the port's typed failures. It does not act on
// ctx: a file's read ends at once, so an interrupt has nothing here to cut
// short. One that waits, a named pipe's, ends on a second interrupt, which
// ends the process at once (decision 31).
func (Files) Read(ctx context.Context, path string) ([]byte, port.ReadFailure) {
	data, err := os.ReadFile(path) //nolint:gosec // caller selects the input path through the Files port
	if err != nil {
		return nil, classifyReadError(path, err)
	}
	return data, nil
}

func classifyReadError(path string, err error) port.ReadFailure {
	if errors.Is(err, fs.ErrNotExist) {
		return &port.Missing{Path: path}
	}
	return &port.Unreadable{Path: path}
}
