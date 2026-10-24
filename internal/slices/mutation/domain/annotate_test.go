package domain

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
)

func TestStripLeavesABlockFollowedByCode(t *testing.T) {
	for _, m := range AnnotationMarkers {
		src := "// " + m[0] + " 1 killed, 0 survived, 0 uncovered\n// " + m[1] + "\nfunc f() {}\n"
		if StripAnnotation(src, "//") != src {
			t.Errorf("removed a block with the markers %q that code follows", m)
		}
	}
}

func TestStripRemovesATrailingCommentWithEitherMarkers(t *testing.T) {
	code := "func f() {}\n"
	for _, m := range AnnotationMarkers {
		src := code + "\n// " + m[0] + " 1 killed, 0 survived, 0 uncovered\n// " + m[1] + "\n"
		if got := StripAnnotation(src, "//"); got != code {
			t.Errorf("markers %q: stripping left %q, want %q", m, got, code)
		}
	}
}

// ts is a TypeScript file whose own namespace is m.
var ts = &lang.File{Spec: lang.Detect("m.ts"), Namespace: "m"}

func TestAnnotationCountsExceptedSurvivorsApart(t *testing.T) {
	snap := Snapshot{Units: []UnitResult{{Namespace: "m", Name: "a", Hash: "h1", Killed: 1, Survived: 2, Mutants: []Mutant{
		{Line: 2, Offset: 20, Original: ">", Replacement: ">=", Outcome: Survived},
		{Line: 3, Offset: 30, Original: "<", Replacement: "<=", Outcome: Survived},
		{Line: 4, Offset: 40, Original: "+", Replacement: "-", Outcome: Killed},
	}}}}
	at := func(hash string, m Mutant) string { return ExceptedKey("m#a", hash, m.Key()) }
	excepted := map[string]string{
		at("h1", snap.Units[0].Mutants[1]): "the loop\n\t only  counts up ",
		// Another hash: an entry for another version of a excepts nothing.
		at("h0", snap.Units[0].Mutants[0]): "stale",
	}
	want := "// itos-cc mutation: 1 killed, 1 survived, 1 excepted, 0 uncovered\n" +
		"// survived: line 2 `>` → `>=` in a\n" +
		"// excepted: line 3 `<` → `<=` in a: the loop only counts up\n" +
		"// end itos-cc mutation\n"
	if got := Annotation(snap, ts, excepted); got != want {
		t.Errorf("annotation:\n%s\nwant:\n%s", got, want)
	}
	// None excepted: the comment reads as it always did.
	want = "// itos-cc mutation: 1 killed, 2 survived, 0 uncovered\n" +
		"// survived: line 2 `>` → `>=` in a\n// survived: line 3 `<` → `<=` in a\n// end itos-cc mutation\n"
	if got := Annotation(snap, ts, nil); got != want {
		t.Errorf("annotation with none excepted:\n%s\nwant:\n%s", got, want)
	}
}

// The comment names a method with its class and a top-level function bare,
// whatever the file's own namespace (slice-46).
func TestCommentNameQualifiesAMethodWithItsClass(t *testing.T) {
	for _, c := range []struct {
		file      string
		namespace string
		want      string
	}{
		{"m", "m", "count"},
		{"m", "m.Board", "Board#count"},
		{"m", "m.Outer.Inner", "Outer.Inner#count"},
		{"", "Board", "Board#count"},
		{"m", "mx.Board", "mx.Board#count"},
	} {
		file := &lang.File{Spec: lang.Detect("m.ts"), Namespace: c.file}
		if got := CommentName(file, UnitResult{Namespace: c.namespace, Name: "count"}); got != c.want {
			t.Errorf("CommentName of %q#count in a file of %q = %q, want %q", c.namespace, c.file, got, c.want)
		}
	}
	snap := Snapshot{Units: []UnitResult{{Namespace: "m.Board", Name: "count", Survived: 1, Mutants: []Mutant{
		{Line: 7, Original: "<", Replacement: "<=", Outcome: Survived},
	}}}}
	want := "// itos-cc mutation: 0 killed, 1 survived, 0 uncovered\n" +
		"// survived: line 7 `<` → `<=` in Board#count\n" +
		"// end itos-cc mutation\n"
	if got := Annotation(snap, ts, nil); got != want {
		t.Errorf("annotation:\n%s\nwant:\n%s", got, want)
	}
	// A file without sites gets no comment.
	if got := Annotation(Snapshot{Units: []UnitResult{{Namespace: "m", Name: "a"}}}, ts, nil); got != "" {
		t.Errorf("annotation of a file without sites: %q, want none", got)
	}
}
