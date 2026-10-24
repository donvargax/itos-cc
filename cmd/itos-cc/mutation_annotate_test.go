package main

import (
	"os"
	"strings"
	"testing"
)

// Beside annotate-excepted, at the end of "Rule: Equivalent mutants
// excepted, each with a reason" in features/mutate.feature, whose ID-MUT-138
// to 140 hold their old tests whole (T-101): a guard that the summary of a
// --since run excepts the survivors of functions it did not judge. It uses
// the module of mutate_since_test.go, as the other scenarios of that rule
// do: Board#place's one mutant is killed, and Board#clear's one mutant,
// `<` → `<=` at src/board.go:11:11, survives whenever it runs and stands for
// the equivalent mutant at src/board.ts:7:19. So where the scenario's comment
// counts 14 killed and names line 7, `<` → `!=` and Board#count, this one
// counts 1 killed and names line 11, `<` → `<=` and Board#clear, as the
// comment names a method (slice-46). Each run here writes the summary
// comment, which the other scenarios turn off.

// annotatedRun runs itos-cc mutation run with args, every mutant run
// whatever coverage says, and the summary comment written.
func annotatedRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--no-coverage", "--workers", "1"}, args...)...)
}

// summaryComment is the summary comment that ends src/board.go, from its
// first line to its last.
func summaryComment(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(inWD(t, boardSource))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	at := strings.LastIndex(src, "// itos-cc mutation:")
	if at < 0 {
		t.Fatalf("%s ends with no summary comment:\n%s", boardSource, src)
	}
	return src[at:]
}

// Under --since, the functions not judged keep their outcomes, and the
// summary comment excepts their survivors as a full run's does: an entry
// holds by its function's hash and its site, which needs no run. A stale
// entry for one excepts nothing there either.
func TestUnderSinceTheSummaryCommentExceptsTheSurvivorsOfFunctionsNotJudged(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	changePlace(t)

	o := annotatedRun(t, "--json", "--since", "base")
	if j := o.json(t).judged(t, boardSource); j == nil || len(*j) != 1 || (*j)[0] != placeID {
		t.Fatalf("judged %v, want place alone\n%s", j, o.stdout)
	}
	want := "// itos-cc mutation: 1 killed, 0 survived, 1 excepted, 0 uncovered\n" +
		"// excepted: line 11 `<` → `<=` in Board#clear: clear is never called\n" +
		"// end itos-cc mutation\n"
	if comment := summaryComment(t); comment != want {
		t.Errorf("the summary comment:\n%s\nwant:\n%s", comment, want)
	}

	// Written for another version of clear, the entry is stale.
	stale := clearException(t, "clear is never called")
	stale.Hash = "0000000000000000"
	writeExceptions(t, "", stale)
	if o := annotatedRun(t, "--since", "base"); o.code != 0 {
		t.Fatalf("with a stale entry for clear: exit %d, want 0: clear is not judged\n%s%s", o.code, o.stdout, o.stderr)
	}
	want = "// itos-cc mutation: 1 killed, 1 survived, 0 uncovered\n" +
		"// survived: line 11 `<` → `<=` in Board#clear\n" +
		"// end itos-cc mutation\n"
	if comment := summaryComment(t); comment != want {
		t.Errorf("with a stale entry for clear, the summary comment:\n%s\nwant:\n%s", comment, want)
	}
}
