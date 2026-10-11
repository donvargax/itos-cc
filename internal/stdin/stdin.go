// Package stdin is the Files adapter for standard input: the reader behind
// the argument -, and the Files it was given for any other path.
//
// The domain knows no paths of the command line and no standard input: it is
// handed bytes through the Files port, and this adapter is the one place the
// - of a command line becomes the reader behind it. It imports nothing of
// ours but the port: cmd gives it the disk adapter as its fallback, so no
// adapter imports another.
package stdin

import (
	"context"
	"io"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// Dash is the argument that names standard input.
const Dash = "-"

// Files reads all of In where the path is Dash, and any other path through
// Fallback.
type Files struct {
	Fallback port.Files
	In       io.Reader
}

var _ port.Files = Files{}

// Read reads path, and refuses any path but Dash through In, so a caller
// that names a file reads it through the fallback and one that names - reads
// a reader. A reader that fails is the environment's, as a file that cannot
// be read is.
//
// Standard input may wait as long as nobody closes it, so the read acts on
// ctx: once ctx is done it ends with port.Cancelled, the input not read
// whole, and a done ctx reads nothing at all. No read of a terminal or a
// pipe can be stopped on every system, so the one under way is left to end
// with the process, which a cancelled command is about to end.
func (f Files) Read(ctx context.Context, path string) ([]byte, port.ReadFailure) {
	if path != Dash {
		return f.Fallback.Read(ctx, path)
	}
	if ctx.Err() != nil {
		return nil, &port.Cancelled{Path: path}
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(f.In)
		read <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, &port.Cancelled{Path: path}
	case r := <-read:
		if r.err != nil {
			return nil, &port.Unreadable{Path: path}
		}
		return r.data, nil
	}
}

// result is what reading In whole gave.
type result struct {
	data []byte
	err  error
}
