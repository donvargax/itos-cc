package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"itos-cc/lang"
)

func TestAnnotateReplacesItsOwnBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.py")
	code := "def a(x):\n    return x > 0\n"
	os.WriteFile(path, []byte(code), 0o644)
	spec := lang.Detect(path)
	snap := Snapshot{Units: []UnitResult{{Name: "a", Killed: 1, Survived: 1, Mutants: []Mutant{
		{Line: 2, Original: ">", Replacement: ">=", Outcome: Survived},
	}}}}
	if err := annotate(path, spec, snap); err != nil {
		t.Fatal(err)
	}
	once, _ := os.ReadFile(path)
	want := code + "\n# itos-cc mutate: 1 killed, 1 survived, 0 uncovered\n" +
		"# survived: line 2 `>` → `>=` in a\n# end itos-cc mutate\n"
	if string(once) != want {
		t.Fatalf("annotated:\n%s\nwant:\n%s", once, want)
	}

	snap.Units[0].Killed, snap.Units[0].Survived, snap.Units[0].Mutants = 2, 0, nil
	annotate(path, spec, snap)
	twice, _ := os.ReadFile(path)
	if strings.Count(string(twice), "itos-cc mutate:") != 1 || !strings.Contains(string(twice), "2 killed, 0 survived") {
		t.Errorf("second run did not replace the block:\n%s", twice)
	}
	if StripAnnotation(string(twice), "#") != code {
		t.Errorf("stripping left %q", StripAnnotation(string(twice), "#"))
	}
}

func TestStripLeavesABlockFollowedByCode(t *testing.T) {
	src := "// itos-cc mutate: 1 killed, 0 survived, 0 uncovered\n// end itos-cc mutate\nfunc f() {}\n"
	if StripAnnotation(src, "//") != src {
		t.Error("removed a block that code follows")
	}
}
