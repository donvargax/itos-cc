package project

import (
	"context"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-cc/lang/port"
)

// Surroundings are lang's Surroundings as this process sees the disk: the
// calls lang made itself before it asked a port (os.Stat, os.ReadFile,
// os.Getwd and filepath.Abs), each answer false where the call failed.
type Surroundings struct{}

var _ port.Surroundings = Surroundings{}

// Exists says whether path can be stat'ed: a path that cannot, for any
// reason, is not there. A stat ends at once, so ctx has nothing to cut
// short.
func (Surroundings) Exists(_ context.Context, path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Read is what the file at path holds, false when it cannot be read. A
// done ctx reads nothing; a read under way runs to its end.
func (Surroundings) Read(ctx context.Context, path string) ([]byte, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	text, err := os.ReadFile(path) //nolint:gosec // lang reads the project files above a source it was given
	if err != nil {
		return nil, false
	}
	return text, true
}

// WorkingDir is os.Getwd's answer, false when it fails.
func (Surroundings) WorkingDir(context.Context) (string, bool) {
	wd, err := os.Getwd()
	return wd, err == nil
}

// Abs is filepath.Abs's answer, false when it fails.
func (Surroundings) Abs(_ context.Context, path string) (string, bool) {
	abs, err := filepath.Abs(path)
	return abs, err == nil
}
