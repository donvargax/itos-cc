// Package port defines what the coverage domain reaches outside itself:
// whether a path a report names is a regular file, and which source it is
// when the report reached it another way, and the sealed set of gaps, why
// coverage meant to be measured was not; and the Measurer the commands
// measure coverage through, which gives the domain's Report or a sealed
// failure (decision 46).
package port

import "context"

// Files is what the report-path matcher asks of the files a report names,
// with paths as the operating system names them, a relative path from the
// working directory. ctx carries only cancellation and deadlines, never a
// value (decision 30); SameFile does not act on it, since nothing it does
// waits, and a caller that was stopped discards what it built.
// porttest.FilesContract holds what every Files, the fake and the disk's,
// must do.
type Files interface {
	// SameFile says whether path names a regular file and, when it does,
	// which of among is that file, reached by the same path or another,
	// such as one through a symlinked directory: the first of among that
	// is, or "" when none is. A path that is not there, or a directory, is
	// no regular file.
	SameFile(ctx context.Context, path string, among []string) (same string, regular bool)
}
