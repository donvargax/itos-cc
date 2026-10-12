package porttest

import (
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/counting/port"
)

// The fake keeps the contract every Files keeps, the disk's included. Its
// directory is a name only: the fake never touches it, and a temporary
// directory's is absolute on every system.
func TestFilesKeepTheContract(t *testing.T) {
	FilesContract(t, func(t *testing.T, files map[string][]byte) (port.Files, string) {
		dir := t.TempDir()
		return Files{Dir: dir, Data: files}, dir
	})
}

func TestReadReturnsAnIndependentCopy(t *testing.T) {
	data := []byte("notes")
	files := Files{Data: map[string][]byte{"input": data}}
	got, failure := files.Read(t.Context(), "input")
	if failure != nil {
		t.Fatal(failure)
	}
	got[0] = 'N'
	if string(files.Data["input"]) != "notes" || string(data) != "notes" {
		t.Errorf("Read() exposed mutable fake state: map=%q source=%q", files.Data["input"], data)
	}
}

// A failure configured for a path is Read's, whatever the path holds, and
// a path rooted with no volume is on the volume of Dir.
func TestReadFailsAsConfigured(t *testing.T) {
	stopped := &port.Cancelled{Path: "/work/notes"}
	files := Files{Dir: "/work", Data: map[string][]byte{"/work/notes": []byte("one")}, Failures: map[string]port.ReadFailure{"/work/notes": stopped}}
	if _, failure := files.Read(t.Context(), "/work/notes"); failure != stopped {
		t.Errorf("Read(/work/notes) = %v, want the configured failure", failure)
	}
	if got, failure := files.Read(t.Context(), "notes"); failure != nil || string(got) != "one" {
		t.Errorf("Read(notes) = %q, %v; want the file at /work/notes", got, failure)
	}
	var missing *port.Missing
	if _, failure := (Files{}).Read(t.Context(), "input"); !errors.As(failure, &missing) {
		t.Errorf("Read(input) of no files = %T %v, want Missing", failure, failure)
	}
}
