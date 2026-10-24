package covrun

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// Disk is the coverage port's Files over the disk, which the runner reads
// its reports from: what a path a report names is, as os.Stat follows it,
// symlinks and all. It does not act on ctx: a stat does not wait.
// porttest.FilesContract holds it to what the fake does.
type Disk struct{}

var _ port.Files = Disk{}

// SameFile says whether path names a regular file and, when it does, the
// first of among that is the same file, as os.SameFile tells, or "".
func (Disk) SameFile(_ context.Context, path string, among []string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	for _, candidate := range among {
		if other, err := os.Stat(candidate); err == nil && os.SameFile(info, other) {
			return candidate, true
		}
	}
	return "", true
}

// Load reads the report at file, in the format the domain's Parse finds in
// its content.
func Load(file string) ([]coverage.Entry, error) {
	f, err := os.Open(file) //nolint:gosec // a report a plan names or the person passed; reading it is the point
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read alone: closing it after decoding loses nothing
	return coverage.Parse(f)
}
