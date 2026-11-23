package processes

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("target full") }

// TestJoinOwnedOutputReportsWhatItCouldNotKeep holds the three things a
// capture's end can say: the bytes reach the caller's target, a target that
// refuses them is named, and a reader's own error is named too.
func TestJoinOwnedOutputReportsWhatItCouldNotKeep(t *testing.T) {
	kept := &bytes.Buffer{}
	for _, c := range []struct {
		name    string
		target  io.Writer
		readErr error
		want    string
	}{
		{name: "handed over", target: &bytes.Buffer{}},
		{name: "kept output takes no copy", target: kept},
		{name: "target refuses", target: failingWriter{}, want: "preserve command output: target full"},
		{name: "reader failed", target: &bytes.Buffer{}, readErr: errors.New("pipe broke"), want: "join command output reader: pipe broke"},
	} {
		t.Run(c.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			done := make(chan capturedOutput, 1)
			done <- capturedOutput{held: bytes.NewBufferString("held"), err: c.readErr}
			err = joinOwnedOutput(ownedOutputCapture{reader: reader, writer: writer, target: c.target, done: done}, kept)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("error = %v; want none", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("error = %v; want it to hold %q", err, c.want)
			}
			if b, ok := c.target.(*bytes.Buffer); ok && c.target != io.Writer(kept) && b.String() != "held" {
				t.Fatalf("target holds %q; want %q", b.String(), "held")
			}
		})
	}
}
