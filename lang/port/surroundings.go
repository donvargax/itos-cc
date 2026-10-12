// Package port defines what lang reaches outside itself to name a file's
// namespace: whether a path is there, what a file holds, the working
// directory, and a path made absolute. lang keeps every language's rules
// (the markers it looks for, the walk up to them, the go.mod module line)
// and asks these questions; it reads nothing itself (decisions 42 and 43).
package port

import "context"

// Surroundings is what surrounds the files lang parses, with paths as the
// operating system names them. ctx carries only cancellation and deadlines,
// never a value (decision 30). An answer lang cannot have is false: lang
// then names the file as it does when the disk gives no answer, and once
// ctx is done it returns the context's error, so no answer given after
// that matters.
type Surroundings interface {
	// Exists says whether path names a file or a directory.
	Exists(ctx context.Context, path string) bool
	// Read is what the file at path holds, false when it cannot be read.
	Read(ctx context.Context, path string) ([]byte, bool)
	// WorkingDir is the working directory, absolute, false when it cannot
	// be told.
	WorkingDir(ctx context.Context) (string, bool)
	// Abs is path made absolute and clean, false when it cannot be.
	Abs(ctx context.Context, path string) (string, bool)
}
