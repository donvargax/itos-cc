package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestAnnotateReplacesItsOwnBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.py")
	code := "def a(x):\n    return x > 0\n"
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := lang.Detect(path)
	snap := mutants.Snapshot{Units: []mutants.UnitResult{{Name: "a", Killed: 1, Survived: 1, Mutants: []mutants.Mutant{
		{Line: 2, Original: ">", Replacement: ">=", Outcome: mutants.Survived},
	}}}}
	if err := (SourceEdits{}).Annotate(t.Context(), path, spec.Comment, mutants.Annotation(snap, spec.Comment, nil)); err != nil {
		t.Fatal(err)
	}
	once, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := code + "\n# itos-cc mutation: 1 killed, 1 survived, 0 uncovered\n" +
		"# survived: line 2 `>` → `>=` in a\n# end itos-cc mutation\n"
	if string(once) != want {
		t.Fatalf("annotated:\n%s\nwant:\n%s", once, want)
	}

	snap.Units[0].Killed, snap.Units[0].Survived, snap.Units[0].Mutants = 2, 0, nil
	if err := (SourceEdits{}).Annotate(t.Context(), path, spec.Comment, mutants.Annotation(snap, spec.Comment, nil)); err != nil {
		t.Fatal(err)
	}
	twice, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(twice), "itos-cc mutation:") != 1 || !strings.Contains(string(twice), "2 killed, 0 survived") {
		t.Errorf("second run did not replace the block:\n%s", twice)
	}
	if mutants.StripAnnotation(string(twice), "#") != code {
		t.Errorf("stripping left %q", mutants.StripAnnotation(string(twice), "#"))
	}
}

// @ID-MUT-145
func TestASummaryCommentWithTheOldMarkersIsReplacedByOneWithTheNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.py")
	code := "def a(x):\n    return x > 0\n"
	old := "\n# itos-cc mutate: 2 killed, 0 survived, 0 uncovered\n# end itos-cc mutate\n"
	if err := os.WriteFile(path, []byte(code+old), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := mutants.Snapshot{Units: []mutants.UnitResult{{Name: "a", Killed: 1, Survived: 1, Mutants: []mutants.Mutant{
		{Line: 2, Original: ">", Replacement: ">=", Outcome: mutants.Survived},
	}}}}
	if err := (SourceEdits{}).Annotate(t.Context(), path, lang.Detect(path).Comment, mutants.Annotation(snap, lang.Detect(path).Comment, nil)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
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
