package main

import (
	"os"
	"strings"
	"testing"
)

// The scenarios of annotate-excepted, at the end of "Rule: Equivalent
// mutants excepted, each with a reason" in features/mutate.feature. They use
// the module of mutate_since_test.go, as the other scenarios of that rule
// do: Board#place's one mutant is killed, and Board#clear's one mutant,
// `<` → `<=` at src/board.go:11:11, survives whenever it runs and stands for
// the equivalent mutant at src/board.ts:7:19. So where the scenario's comment
// counts 14 killed and names line 7, `<` → `!=` and Board#count, this one
// counts 1 killed and names line 11, `<` → `<=` and clear, the name a
// survived line gives the function. Each run here writes the summary
// comment, which the other scenarios turn off.

// annotatedRun runs itos-cc mutation run with args, every mutant run
// whatever coverage says, and the summary comment written.
func annotatedRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "run", "--no-coverage", "--workers", "1"}, args...)...)
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

// @ID-MUT-138
func TestTheSummaryCommentCountsAnExceptedSurvivorApartWithItsReason(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))

	if o := annotatedRun(t, boardSource); o.code != 0 {
		t.Fatalf("exit %d, want 0: clear's survivor is excepted\n%s%s", o.code, o.stdout, o.stderr)
	}
	data, _ := os.ReadFile(inWD(t, boardSource))
	want := "}\n\n// itos-cc mutation: 1 killed, 0 survived, 1 excepted, 0 uncovered\n" +
		"// excepted: line 11 `<` → `<=` in clear: clear is never called\n" +
		"// end itos-cc mutation\n"
	if !strings.HasSuffix(string(data), want) {
		t.Errorf("%s:\n%s\nwant it to end with:\n%s", boardSource, data, want)
	}
}

// @ID-MUT-139
func TestAStaleEntrysSurvivorIsListedAsSurvived(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	// clear changed, and its mutant at the entry's site still survives.
	edit(t, boardSource, "i < 3", "i < 4")

	o := annotatedRun(t, "--json", boardSource)
	if m := mutantOf(t, o, clearID); m.Outcome != "survived" || m.Line != clearLine || m.Column != clearColumn {
		t.Fatalf("clear's mutant %+v, want it to survive at the entry's site", m)
	}
	comment := summaryComment(t)
	for _, want := range []string{
		"// itos-cc mutation: 1 killed, 1 survived, 0 uncovered\n",
		"\n// survived: line 11 `<` → `<=` in clear\n",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("the summary comment:\n%s\nwant it to hold %q", comment, want)
		}
	}
	if strings.Contains(comment, "excepted") {
		t.Errorf("the summary comment:\n%s\nwant no \"excepted\" count and no \"excepted:\" line", comment)
	}
}

// @ID-MUT-140
func TestAReasonOverSeveralLinesIsWrittenOnOneCommentLine(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is\n  never called"))

	if o := annotatedRun(t, boardSource); o.code != 0 {
		t.Fatalf("exit %d, want 0: clear's survivor is excepted\n%s%s", o.code, o.stdout, o.stderr)
	}
	comment := summaryComment(t)
	lines := strings.Split(strings.TrimSuffix(comment, "\n"), "\n")
	const want = "// excepted: line 11 `<` → `<=` in clear: clear is never called"
	if !strings.Contains(comment, "\n"+want+"\n") {
		t.Errorf("the summary comment:\n%s\nwant the line %q", comment, want)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "//") {
			t.Errorf("the summary comment:\n%s\nholds the line %q, which does not begin with \"//\"", comment, l)
		}
	}
}

// Under --since, the functions not judged keep their outcomes, and the
// summary comment excepts their survivors as a full run's does: an entry
// holds by its function's hash and its site, which needs no run. A stale
// entry for one excepts nothing there either.
func TestUnderSinceTheSummaryCommentExceptsTheSurvivorsOfFunctionsNotJudged(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	changePlace(t)

	o := annotatedRun(t, "--json", "--since", "base")
	if j := o.json(t).judged(t, boardSource); j == nil || len(*j) != 1 || (*j)[0] != placeID {
		t.Fatalf("judged %v, want place alone\n%s", j, o.stdout)
	}
	want := "// itos-cc mutation: 1 killed, 0 survived, 1 excepted, 0 uncovered\n" +
		"// excepted: line 11 `<` → `<=` in clear: clear is never called\n" +
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
		"// survived: line 11 `<` → `<=` in clear\n" +
		"// end itos-cc mutation\n"
	if comment := summaryComment(t); comment != want {
		t.Errorf("with a stale entry for clear, the summary comment:\n%s\nwant:\n%s", comment, want)
	}
}
