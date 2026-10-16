package mutants

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

func TestEvidenceKeepsExecutableBlocksAndRejectsAnotherFunction(t *testing.T) {
	f := parse(t, "main.go", "package main\nfunc Value(x int) bool { if x > 0 { return true }; return false }\nfunc Empty() {}\n")
	u := f.Units[0]
	blocks := []coverage.GoBlock{
		{Span: "2.24,2.34", Line: 2, Column: 24, Weight: 1, Covered: true},
		{Span: "2.35,2.48", Line: 2, Column: 35, Weight: 1},
		{Span: "2.50,2.62", Line: 2, Column: 50, Weight: 1, Covered: true},
		{Span: "2.63,2.64", Line: 2, Column: 63, Weight: 0},
		{Span: "3.1,3.15", Line: 3, Column: 1, Weight: 1},
	}
	inputs := map[string]string{"main.go": "sha"}
	producer := CoverageProducer("go", false)
	report := coverage.FromGoBlocks(map[string][]coverage.GoBlock{f.Path: blocks})
	e := CoverageEvidenceFor("main.go", f, u, report, producer, inputs)
	if !e.Complete || len(e.Blocks) != 3 || e.File != "main.go" || e.Function != "main#Value" || e.Hash != UnitHash(f, u) || e.Blocks[1].Covered {
		t.Fatalf("evidence %+v", e)
	}
	inputs["main.go"] = "changed"
	if e.Inputs["main.go"] != "sha" {
		t.Fatal("evidence aliases future inputs")
	}
	admitted := FreshUnit{Path: "main.go", Function: "main#Value", Hash: UnitHash(f, u), StartLine: u.StartLine, EndLine: u.EndLine}
	frozen, err := FreshGoCoverageEvidence(f, admitted, blocks, producer, nil)
	if err != nil || !reflect.DeepEqual(frozen.Blocks, e.Blocks) {
		t.Fatalf("frozen %+v, %v", frozen, err)
	}
	admitted.Hash = "changed"
	if _, err := FreshGoCoverageEvidence(f, admitted, blocks, producer, nil); err == nil {
		t.Fatal("changed admitted identity accepted")
	}
	if empty := GoCoverageEvidenceFor("main.go", f, f.Units[1], nil, producer, nil); !empty.Complete || len(empty.Blocks) != 0 {
		t.Fatal(empty)
	}
	if missing := GoCoverageEvidenceFor("main.go", f, u, nil, producer, nil); missing.Complete {
		t.Fatal("executable function without inventory admitted")
	}
	if GoBlockInUnit(nil, blocks[0]) || GoHasExecutableStatements(nil) {
		t.Fatal("absent syntax is executable")
	}
	py := parse(t, "calc.py", "def value(x):\n    return x > 0\n")
	pyReport := coverage.FromLines("python", map[string][]coverage.Line{py.Path: {{Line: 2, Covered: true}}})
	if evidence := CoverageEvidenceFor("calc.py", py, py.Units[0], pyReport, "p", nil); !evidence.Complete || len(evidence.Blocks) != 1 || !evidence.Blocks[0].Covered {
		t.Fatal(evidence)
	}
}

func TestStrictCoverageAdmitsOnlyCompleteCurrentProducerEvidence(t *testing.T) {
	for _, language := range []string{"go", "python", "typescript", "kotlin"} {
		t.Run(language, func(t *testing.T) {
			producer := CoverageProducer(language, false)
			if language == "typescript" {
				producer = TypeScriptCoverageProducer("vitest", false)
			}
			if language == "kotlin" {
				producer = KotlinCoverageProducer("jacoco", false)
			}
			if !StrictCoverage(language) {
				t.Fatal(language)
			}
			inputs := map[string]string{"source": "h", "@producer": "opaque"}
			e := CoverageEvidence{Version: EvidenceVersion(language), Language: EvidenceKey(language), File: "source", Function: "n#f", Hash: "hash", Producer: producer, Complete: true, Inputs: inputs, Blocks: []CoverageBlock{{Span: "3", Line: 3, Covered: true}}}
			check := CoverageCheck{State: Missing, Language: language, Function: "n#f"}
			got := CoverageFreshness(check, "source", "hash", &e, map[string]string{"source": "h", "@producer": "irrelevant here"})
			if got.State != Fresh || got.Evidence != &e || !reflect.DeepEqual(got.Blocks, e.Blocks) {
				t.Fatal(got)
			}
			for _, change := range []string{"incomplete", "version", "file", "function", "hash", "language"} {
				bad := e
				switch change {
				case "incomplete":
					bad.Complete = false
				case "version":
					bad.Version++
				case "file":
					bad.File = "other"
				case "function":
					bad.Function = "other"
				case "hash":
					bad.Hash = "other"
				case "language":
					bad.Language = "other"
				}
				if got := CoverageFreshness(check, "source", "hash", &bad, inputs); got.State != Missing || got.Evidence != nil {
					t.Fatalf("%s: %+v", change, got)
				}
			}
			if got := CoverageFreshness(check, "source", "hash", nil, inputs); got.State != Missing {
				t.Fatal(got)
			}
			stale := CoverageFreshness(check, "source", "hash", &e, map[string]string{"source": "new", "added": "x"})
			if stale.State != Stale || !slices.Equal(stale.Changed, []string{"added", "source"}) {
				t.Fatal(stale)
			}
			e.Producer = "unknown"
			if got := CoverageFreshness(check, "source", "hash", &e, inputs); got.State != Stale || !slices.Equal(got.Changed, []string{"coverage producer/options"}) {
				t.Fatal(got)
			}
		})
	}
	if StrictCoverage("unknown") || ValidCoverageProducer("go", "") || CoverageProducer("unknown", false) != "" {
		t.Fatal("unknown language or producer admitted")
	}
	if !strings.Contains(CoverageProducer("go", true), "scope=all-tests") || !strings.Contains(CoverageProducer("python", true), "scope=all-tests") {
		t.Fatal("whole-suite producer lost")
	}
}

func TestCommandsAndConfiguredSelectorsRetainTheirActualIdentity(t *testing.T) {
	empty := Command{}
	if !empty.RunsNothing() || empty.String() != "" {
		t.Fatal(empty)
	}
	c := Command{Root: "root", Dir: "pkg", Args: []string{"go", "test"}}
	if c.RunsNothing() || c.String() != "go test" || c.Key() == (Command{Root: "root", Dir: "other", Args: c.Args}).Key() {
		t.Fatal(c)
	}
	c.Shell = "go version"
	if c.String() != "go version" || c.RunsNothing() {
		t.Fatal(c)
	}
	tests := Tests{Run: "go test -run '{pattern}'", IDsPattern: "^({ids})$", Each: "Test{id}", Sep: "|"}
	if tests.Select([]string{"A", "B"}) != "go test -run '^(TestA|TestB)$'" || tests.All([]string{"A"}) != "go test -run '^(TestA)$'" {
		t.Fatal(tests)
	}
	tests.Whole = "all"
	if tests.All(nil) != "all" {
		t.Fatal(tests)
	}
	e := Exception{File: "a", Function: "n#f", LineInFunction: 1, Column: 2, Original: "0", Replacement: "1"}
	other := e
	other.Reason = "reviewed"
	other.Hash = "changed"
	if !e.SameSite(other) {
		t.Fatal("review text/hash do not identify the site")
	}
	other.Column++
	if e.SameSite(other) {
		t.Fatal("different site accepted")
	}
}

func TestCommittedRanksAndDiffHunksAreIndependentOfExecution(t *testing.T) {
	if FreshRank("seed", "a.go", "id") == FreshRank("other", "a.go", "id") || FreshRank("seed", "a.go", "id") == FreshRank("seed", "b.go", "id") {
		t.Fatal("rank does not distinguish inputs")
	}
	candidates := []FreshCandidate{{Identity: "z", Rank: "a"}, {Identity: "a", Rank: "a"}, {Identity: "first", Rank: "0"}}
	SortFreshCandidates(candidates)
	if candidates[0].Identity != "first" || candidates[1].Identity != "a" {
		t.Fatal(candidates)
	}
	ranges := HunkRanges("unrelated\n@@ -1 +3 @@\n@@ -4,2 +7,3 @@\n@@ -9 +12,0 @@\n@@ invalid\n@@ -1 +bad @@\n@@ -1 +3,bad @@\n@@ -1 malformed @@\n")
	want := []LineRange{{3, 3, false}, {7, 9, false}, {12, 13, true}}
	if !reflect.DeepEqual(ranges, want) || !OverlapsRanges(ranges, 8, 8) || OverlapsRanges(ranges, 4, 6) {
		t.Fatal(ranges)
	}
}

func TestFailedSelectionsAreRecordedOnceByTheirSelectedIDs(t *testing.T) {
	result := FileResult{Baseline: BaselineNotRun}
	result.AddFailed(&FailedSelection{IDs: []string{"b", "a"}, Command: "run a b", Output: "failed"})
	result.AddFailed(&FailedSelection{IDs: []string{"a", "b"}, Command: "run b a", Output: "same selection"})
	result.AddFailed(&FailedSelection{IDs: []string{"c"}, Command: "run c", Output: "other failure"})
	if len(result.FailedSelections) != 2 || result.FailedSelections[0].Output != "failed" || result.FailedSelections[1].IDs[0] != "c" {
		t.Fatal(result)
	}
}
