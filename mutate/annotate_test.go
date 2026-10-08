package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/lang"
)

func TestAnnotateReplacesItsOwnBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.py")
	code := "def a(x):\n    return x > 0\n"
	os.WriteFile(path, []byte(code), 0o644)
	spec := lang.Detect(path)
	snap := Snapshot{Units: []UnitResult{{Name: "a", Killed: 1, Survived: 1, Mutants: []Mutant{
		{Line: 2, Original: ">", Replacement: ">=", Outcome: Survived},
	}}}}
	if err := annotate(path, spec, snap, nil); err != nil {
		t.Fatal(err)
	}
	once, _ := os.ReadFile(path)
	want := code + "\n# itos-cc mutate: 1 killed, 1 survived, 0 uncovered\n" +
		"# survived: line 2 `>` → `>=` in a\n# end itos-cc mutate\n"
	if string(once) != want {
		t.Fatalf("annotated:\n%s\nwant:\n%s", once, want)
	}

	snap.Units[0].Killed, snap.Units[0].Survived, snap.Units[0].Mutants = 2, 0, nil
	annotate(path, spec, snap, nil)
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

func TestAnnotationCountsExceptedSurvivorsApart(t *testing.T) {
	snap := Snapshot{Units: []UnitResult{{Namespace: "m", Name: "a", Hash: "h1", Killed: 1, Survived: 2, Mutants: []Mutant{
		{Line: 2, Offset: 20, Original: ">", Replacement: ">=", Outcome: Survived},
		{Line: 3, Offset: 30, Original: "<", Replacement: "<=", Outcome: Survived},
		{Line: 4, Offset: 40, Original: "+", Replacement: "-", Outcome: Killed},
	}}}}
	at := func(hash string, m Mutant) string { return exceptedKey("m#a", hash, m.key()) }
	excepted := map[string]string{
		at("h1", snap.Units[0].Mutants[1]): "the loop\n\t only  counts up ",
		// Another hash: an entry for another version of a excepts nothing.
		at("h0", snap.Units[0].Mutants[0]): "stale",
	}
	want := "// itos-cc mutate: 1 killed, 1 survived, 1 excepted, 0 uncovered\n" +
		"// survived: line 2 `>` → `>=` in a\n" +
		"// excepted: line 3 `<` → `<=` in a: the loop only counts up\n" +
		"// end itos-cc mutate\n"
	if got := Annotation(snap, "//", excepted); got != want {
		t.Errorf("annotation:\n%s\nwant:\n%s", got, want)
	}
	// None excepted: the comment reads as it always did.
	want = "// itos-cc mutate: 1 killed, 2 survived, 0 uncovered\n" +
		"// survived: line 2 `>` → `>=` in a\n// survived: line 3 `<` → `<=` in a\n// end itos-cc mutate\n"
	if got := Annotation(snap, "//", nil); got != want {
		t.Errorf("annotation with none excepted:\n%s\nwant:\n%s", got, want)
	}
}

// @ID-MUT-145
func TestASummaryCommentWithTheOldMarkersIsReplacedByOneWithTheNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.py")
	code := "def a(x):\n    return x > 0\n"
	old := "\n# itos-cc mutate: 2 killed, 0 survived, 0 uncovered\n# end itos-cc mutate\n"
	os.WriteFile(path, []byte(code+old), 0o644)
	snap := Snapshot{Units: []UnitResult{{Name: "a", Killed: 1, Survived: 1, Mutants: []Mutant{
		{Line: 2, Original: ">", Replacement: ">=", Outcome: Survived},
	}}}}
	if err := annotate(path, lang.Detect(path), snap, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	src := string(data)
	if strings.Count(src, "\n# itos-cc mutat") != 1 || strings.Count(src, "\n# end itos-cc mutat") != 1 {
		t.Fatalf("%s:\n%s\nwant exactly one summary comment", path, src)
	}
	opening := strings.Index(src, "# itos-cc mutat")
	comment := src[opening:]
	if !strings.HasPrefix(comment, "# itos-cc mutation:") || !strings.HasSuffix(comment, "\n# end itos-cc mutation\n") {
		t.Errorf("the summary comment:\n%s\nwant it to open \"# itos-cc mutation:\" and close \"# end itos-cc mutation\"", comment)
	}
}
