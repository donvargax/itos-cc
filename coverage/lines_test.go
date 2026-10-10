package coverage

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A successful plan proves the lines of the files its report names, and
// leaves those it never names unloaded; a failed one proves nothing.
func TestLinesNeedAPlanThatMeasuredItsLanguage(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py")
	entries, err := ParseLCOV(strings.NewReader("SF:a.py\nDA:2,1\nDA:3,0\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := Build([]string{a, b}, dir, entries)
	Plan{Language: "python", Dir: dir, Sources: []string{a, b}}.prove(r)
	if lines, ok := r.Lines(a); !ok || !reflect.DeepEqual(lines, []Line{{Line: 2, Covered: true}, {Line: 3}}) {
		t.Errorf("a's lines %v %v, want 2 covered and 3 not, proven", lines, ok)
	}
	if lines, ok := r.Lines(b); ok || lines != nil {
		t.Errorf("b's lines %v %v, want none proven", lines, ok)
	}
	if got := r.Unloaded("python"); !reflect.DeepEqual(got, []string{b}) {
		t.Errorf("unloaded %v, want b alone", got)
	}
	failed := Build([]string{a}, dir, entries)
	if _, ok := failed.Lines(a); ok {
		t.Error("a report no successful plan proved proves a's lines")
	}
	merged := Merge(&Report{}, r)
	if _, ok := merged.Lines(a); !ok || !reflect.DeepEqual(merged.Unloaded("python"), []string{b}) {
		t.Error("merging lost what the plan proved")
	}
}

// coverage.py's own analysis lists the executable lines of a file no test
// loaded, running no test, all of them unexecuted.
func TestInventoryListsTheExecutableLinesOfUnloadedPythonFiles(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil || exec.Command("python3", "-c", "import coverage").Run() != nil {
		t.Skip("python3 with coverage.py is not installed")
	}
	dir := t.TempDir()
	lonely := filepath.Join(dir, "lonely.py")
	text := "\"\"\"Doc.\"\"\"\n\n\ndef lonely(name):\n    \"\"\"Doc.\"\"\"\n    label = name.strip()\n    return label  # pragma: no cover\n"
	if err := os.WriteFile(lonely, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	r := (Plan{Language: "python", Dir: dir, Sources: []string{lonely}, Unreached: true}).unreached(io.Discard)
	calls, err := r.Inventory(context.Background(), "python", io.Discard, nil)
	if err != nil || len(calls) != 1 {
		t.Fatalf("inventory: %v, calls %v", err, calls)
	}
	lines, ok := r.Lines(lonely)
	if want := []Line{{Line: 4}, {Line: 6}}; !ok || !reflect.DeepEqual(lines, want) {
		t.Errorf("lonely's lines %v %v, want %v proven", lines, ok, want)
	}
	if got := r.Unloaded("python"); len(got) != 0 {
		t.Errorf("unloaded after the inventory %v, want none", got)
	}
}
