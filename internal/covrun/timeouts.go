package covrun

import (
	"bytes"
	"io"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// longestLine is the most of one line timeoutWatch holds while it waits for
// the line's end: go test's timeout and FAIL lines are short, so a longer
// line is dropped unread rather than held whole.
const longestLine = 4096

// timeoutWatch passes a Go coverage command's output on to log, nil for
// none, reading each whole line for the packages go test stopped at its
// limit (coverage.GoTimeouts). exec.Cmd calls Write from one goroutine at a
// time when it is both Stdout and Stderr.
type timeoutWatch struct {
	log     io.Writer
	partial []byte
	seen    coverage.GoTimeouts
}

func (t *timeoutWatch) Write(p []byte) (int, error) {
	t.partial = append(t.partial, p...)
	for {
		end := bytes.IndexByte(t.partial, '\n')
		if end < 0 {
			break
		}
		t.seen.Line(string(t.partial[:end]))
		t.partial = t.partial[end+1:]
	}
	if len(t.partial) > longestLine {
		t.partial = t.partial[:0]
	}
	if t.log == nil {
		return len(p), nil
	}
	return t.log.Write(p)
}

// found is each package the output named as timed out, none for a nil
// watch, a command that is not Go's.
func (t *timeoutWatch) found() []coverage.GoTimeout {
	if t == nil {
		return nil
	}
	return t.seen.Found()
}
