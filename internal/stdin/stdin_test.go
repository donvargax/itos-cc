package stdin_test

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/counting"
	"github.com/donvargax/itos-cc/internal/counting/port"
	"github.com/donvargax/itos-cc/internal/counting/port/porttest"
	"github.com/donvargax/itos-cc/internal/stdin"
)

// Standard input is read whole, as a file is: the counts are the domain's,
// whichever the path came from.
func TestFilesReadStandardInputForTheDash(t *testing.T) {
	files := stdin.Files{Fallback: porttest.Files{}, In: strings.NewReader("one two\nthree")}
	got, err := counting.File(t.Context(), files, "-")
	if err != nil || got != (counting.Result{Lines: 2, Words: 3}) {
		t.Fatalf("File(-, …) = %+v, %v; want 2 lines and 3 words", got, err)
	}
}

// A named path is the fallback's, never the reader's: cmd gives the disk.
func TestFilesReadANamedPathThroughTheFallback(t *testing.T) {
	files := stdin.Files{
		Fallback: porttest.Files{Data: map[string][]byte{"notes": []byte("one two")}},
		In:       strings.NewReader("ignored"),
	}
	got, err := counting.File(t.Context(), files, "notes")
	if err != nil || got != (counting.Result{Lines: 1, Words: 2}) {
		t.Fatalf("File(notes, …) = %+v, %v; want 1 line and 2 words", got, err)
	}
}

// A path the fallback has no file for is its failure, the reader's never
// spoken for: the argument named a file, so it is a missing file.
func TestFilesNameAMissingFileThroughTheFallback(t *testing.T) {
	files := stdin.Files{Fallback: porttest.Files{}, In: strings.NewReader("data")}
	_, err := counting.File(t.Context(), files, "absent")
	var missing *port.Missing
	if !errors.As(err, &missing) {
		t.Fatalf("File(absent, …) = %T %v, want typed Missing", err, err)
	}
}

// A reader that fails is the environment's: unreadable, as a file that
// cannot be read is. No permission bit is assumed, so this holds wherever
// the reader fails.
func TestFilesReportAFailingReaderAsUnreadable(t *testing.T) {
	files := stdin.Files{Fallback: porttest.Files{}, In: failingReader{}}
	_, err := counting.File(t.Context(), files, "-")
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) || unreadable.Path != "-" {
		t.Fatalf("File(-, failing reader) = %T %v, want typed Unreadable for -", err, err)
	}
}

// A read waiting on an input nobody closes ends once its context is done,
// as an interrupt makes it: cancelled, the input not read whole, so the
// command stops promptly instead of waiting for an end that never comes.
func TestFilesEndAWaitingReadWhenTheContextIsDone(t *testing.T) {
	input, writer := io.Pipe()
	defer func() { _ = writer.Close() }() // ends the read the adapter left under way
	ctx, cancel := context.WithCancel(t.Context())
	files := stdin.Files{Fallback: porttest.Files{}, In: input}
	read := make(chan error, 1)
	go func() {
		_, err := counting.File(ctx, files, "-")
		read <- err
	}()
	if _, err := writer.Write([]byte("one two")); err != nil { // the read is under way
		t.Fatal(err)
	}
	cancel()
	var cancelled *port.Cancelled
	if err := <-read; !errors.As(err, &cancelled) || cancelled.Path != "-" {
		t.Fatalf("File(-, open input) after cancel = %T %v, want typed Cancelled for -", err, err)
	}
}

// A cancelled read leaves nothing behind once its input ends: the read it
// left under way finishes, and its goroutine with it, never blocked on
// handing over a result nobody waits for, so a program that lives on after
// a cancelled read does not keep one goroutine for each.
func TestACancelledReadLeavesNoGoroutineOnceItsInputEnds(t *testing.T) {
	before := runtime.NumGoroutine()
	input, writer := io.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	files := stdin.Files{Fallback: porttest.Files{}, In: input}
	read := make(chan error, 1)
	go func() {
		_, err := counting.File(ctx, files, "-")
		read <- err
	}()
	if _, err := writer.Write([]byte("one")); err != nil { // the read is under way
		t.Fatal(err)
	}
	cancel()
	var cancelled *port.Cancelled
	if err := <-read; !errors.As(err, &cancelled) {
		t.Fatalf("File(-, open input) after cancel = %T %v, want typed Cancelled", err, err)
	}
	_ = writer.Close()
	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines remain of the cancelled read, want none", runtime.NumGoroutine()-before)
		}
		time.Sleep(time.Millisecond)
	}
}

// A context already done reads nothing, even of an input that is ready: the
// command was interrupted before it asked, and a ready input must not race
// the interrupt into counts.
func TestFilesReadNothingUnderADoneContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	files := stdin.Files{Fallback: porttest.Files{}, In: strings.NewReader("one two")}
	for range 100 {
		_, err := counting.File(ctx, files, "-")
		var cancelled *port.Cancelled
		if !errors.As(err, &cancelled) {
			t.Fatalf("File(-, ready input) under a done context = %T %v, want typed Cancelled", err, err)
		}
	}
}

// failingReader is a reader that always fails, an environment the tests can
// make wherever a file cannot be made unreadable.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("cannot read") }
