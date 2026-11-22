package mutation

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// TestPlainLinesNameFilesSlashSeparated holds every plain line that names
// a file to the one form a person reads, src/board.go, when the system
// names it src\board.go (T-235, after bug-9, bug-12 and slice-68): one case
// per line, each with the separator of Windows given as a value, so a Linux
// run sees what Windows prints.
func TestPlainLinesNameFilesSlashSeparated(t *testing.T) {
	t.Parallel()
	const file = `src\board.go`
	entry := mutants.Exception{File: `board\board.go`, Function: "board#Count", Hash: "ab", LineInFunction: 2, Column: 11,
		Original: ">", Replacement: ">=", Reason: "reviewed"}
	now := entry
	now.Hash = "cd"
	survivor := mutants.Mutant{Line: 5, Column: 9, Original: ">", Replacement: ">=", Outcome: mutants.Survived}
	failed := mutants.FailedSelection{IDs: []string{"ID-A-02"}, Command: "go test -run ID-A-02 ./e2e", ExitCode: 1, Output: "FAIL"}
	moved := mutants.StaleException{Exception: mutants.Exception{Function: "board#clear", Original: "0", Replacement: "1", Column: 3},
		Why: mutants.ExceptionMoved, NewFile: `src\game\board.go`}
	mismatch := mutants.SampledMutant{Site: mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="},
		Function: "board#place", Recorded: mutants.Killed, Outcome: mutants.Survived, Outcomes: slices.Repeat([]string{mutants.Survived}, mutants.Reruns+1)}
	flaky := mutants.SampledMutant{Site: mutants.Site{Line: 9, Column: 3, Original: "0", Replacement: "1"},
		Function: "board#clear", Recorded: mutants.Killed, Outcome: mutants.Survived, Outcomes: []string{mutants.Survived, mutants.Killed}}
	stoppedAt := &mutants.Stop{Rule: "mutation.survived", File: file, Function: "board#place",
		Site: &mutants.Site{Line: 5, Column: 9, Original: ">", Replacement: ">="}}
	found := func(results ...mutants.FileResult) RunFound { return RunFound{Results: results} }
	run := func(p *Presentation, found RunFound, stopped bool) {
		result := runResult{Work: &runWork{}}
		Run{}.present(t.Context(), p, found, &result, stopped)
	}
	for _, tc := range []struct {
		name string
		do   func(p *Presentation)
		want string
	}{
		{"run: the failing baseline", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, BaselineFailed: true, BaselineOutput: mutants.Output{Text: "FAILED\n"}}), false)
		}, "baseline tests of src/board.go fail; snapshot not updated"},
		{"run: the failing listed tests", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, FailedSelections: []mutants.FailedSelection{failed}}), false)
		}, "listed tests the mutants of src/board.go run fail without any mutant; snapshot not updated"},
		{"run: the file's counts", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, Ran: 1}), false)
		}, "0 killed, 0 survived, 0 uncovered in src/board.go (ran 1, reused 0)"},
		{"run: a survivor", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, Ran: 1, Snapshot: mutants.Snapshot{Units: []mutants.UnitResult{
				{Namespace: "board", Name: "place", Survived: 1, Mutants: []mutants.Mutant{survivor}}}}}), false)
		}, "  survived `>` → `>=` in board#place, src/board.go line 5 column 9"},
		{"run: the file an exception moved to", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, StaleExceptions: []mutants.StaleException{moved}}), false)
		}, "  exception-stale `0` → `1` in board#clear, src/board.go (moved to src/game/board.go)"},
		{"run: a stop's file", func(p *Presentation) {
			p.stopped(stoppedAt, &runResult{Work: &runWork{}})
		}, "stopped early at the first actionable failure, mutation.survived `>` → `>=` in board#place, src/board.go line 5 column 9: " +
			"0 completed, 0 cancelled, 0 unattempted, 0 blocked; a file it cut short keeps its source as it was, and its snapshot only what was judged"},
		{"run: a file cut short", func(p *Presentation) {
			run(p, found(mutants.FileResult{Rel: file, Incomplete: true, Ran: 1}), true)
		}, "stopped early, snapshot not updated: 0 killed, 0 survived, 0 uncovered in src/board.go (ran 1, reused 0); 0 cancelled, 0 unattempted; cache incomplete"},
		{"check: a file's counts", func(p *Presentation) {
			p.FileChecked(file, map[string]int{mutants.SnapshotFresh: 2}, nil)
		}, "2 fresh, 0 stale, 0 missing in src/board.go"},
		{"check: a missing function", func(p *Presentation) {
			p.Function(file, mutants.FunctionCheck{Function: "board#reset", StartLine: 14}, "mutation.missing", "has no mutation results")
		}, "  missing src/board.go:14 in board#reset"},
		{"check: a left-out function", func(p *Presentation) {
			p.LeftOut(file, mutants.FunctionCheck{Function: "board#clear", StartLine: 9, LeftOut: 1})
		}, "  left out src/board.go:9 in board#clear: 1 recorded mutant at sites gone, which the next mutation run drops"},
		{"check: strict coverage's finding", func(p *Presentation) {
			p.CoverageProblem("go", file, "board#place", 12, "mutation.uncovered-statement", "has an uncovered statement")
		}, "  uncovered-statement src/board.go:12 in board#place"},
		{"check: an orphan", func(p *Presentation) {
			p.Orphans([]Orphan{{File: `src\gone.go`, Snapshot: "src/gone.go.json", Why: "gone"}}, "is an orphan, which mutation run removes")
		}, "snapshot src/gone.go.json is an orphan, which mutation run removes: its source src/gone.go is gone"},
		{"sample: a file's counts", func(p *Presentation) {
			p.sampledFile(mutants.SampledFile{Rel: file, Mutants: []mutants.SampledMutant{mismatch}}, &sampleResult{}, map[string]bool{})
		}, "1 sampled, 1 mismatched, 0 flaky in src/board.go"},
		{"sample: a mismatch", func(p *Presentation) {
			p.sampledFile(mutants.SampledFile{Rel: file, Mutants: []mutants.SampledMutant{mismatch}}, &sampleResult{}, map[string]bool{})
		}, "  mismatch `>` → `>=` in board#place, src/board.go line 5 column 9: recorded killed, now survived"},
		{"sample: a flaky mutant", func(p *Presentation) {
			p.sampledFile(mutants.SampledFile{Rel: file, Mutants: []mutants.SampledMutant{flaky}}, &sampleResult{}, map[string]bool{})
		}, "  flaky `0` → `1` in board#clear, src/board.go line 9 column 3: recorded killed, now survived, killed"},
		{"sample: the failing baseline", func(p *Presentation) {
			p.sampledFile(mutants.SampledFile{Rel: file, BaselineFailed: true, BaselineOutput: mutants.Output{Text: "FAILED\n"}}, &sampleResult{}, map[string]bool{})
		}, "baseline tests of src/board.go fail; no sampled mutant ran"},
		{"sample: the failing listed tests", func(p *Presentation) {
			p.sampledFile(mutants.SampledFile{Rel: file, FailedSelections: []mutants.FailedSelection{failed}}, &sampleResult{}, map[string]bool{})
		}, "listed tests the sampled mutants of src/board.go run fail without any mutant; no sampled mutant is judged"},
		{"a file another system builds", func(p *Presentation) {
			p.sayNotBuilt([]mutants.NotBuilt{{Path: file, Platform: mutants.Platform{OS: "linux", Arch: "amd64"}}})
		}, "itos-cc: not built on linux/amd64, so src/board.go is not judged here"},
		{"a counted run's site", func(p *Presentation) {
			p.printCounted(&countedResult{Sampling: countedSampling{Commit: "abc", Seed: "s", Algorithm: "a", Budget: 1, Eligible: 1, Selected: 1, Executed: 1},
				Selected: []countedSite{{File: file, Line: 5, Column: 9, Function: "board#place", Original: ">", Replacement: ">=",
					State: "judged", Outcome: mutants.Survived}}})
		}, "  survived `>` → `>=` in board#place, src/board.go line 5 column 9"},
		{"except: the excepted line", func(p *Presentation) {
			Except{}.excepted(p, ExceptFound{Exception: entry, Path: `board\board.go`, Line: 6, Column: 11})
		}, "excepted board/board.go:6:11 `>` → `>=` in board#Count in itos-cc.yaml"},
		{"except: the excepted-again line", func(p *Presentation) {
			Except{}.excepted(p, ExceptFound{Exception: entry, Replaced: true, Path: `board\board.go`, Line: 6, Column: 11})
		}, "excepted again board/board.go:6:11 `>` → `>=` in board#Count in itos-cc.yaml"},
		{"except: the renewed line", func(p *Presentation) {
			Except{}.renewed(p, RenewFound{Renewed: []mutants.Renewal{{Old: entry, New: now, Line: 6, Match: "line"}}})
		}, "renewed board/board.go:6:11 `>` → `>=` in board#Count (hash ab → cd) in itos-cc.yaml"},
		{"except: the not-renewed lines", func(p *Presentation) {
			Except{}.renewed(p, RenewFound{Left: []mutants.Unrenewed{{Exception: entry, Why: mutants.ExceptionKilled, Line: 6}, {Exception: entry, Why: mutants.ExceptionGone}}})
		}, "  not renewed board/board.go `>` → `>=` in board#Count (gone)"},
		{"except: the pruned line", func(p *Presentation) {
			Except{}.pruned(p, PruneFound{Pruned: []mutants.StaleException{{Exception: entry, Why: mutants.ExceptionKilled, Line: 6}}})
		}, "pruned board/board.go:6:11 `>` → `>=` in board#Count (killed) from itos-cc.yaml"},
		{"except: the rehashed line", func(p *Presentation) {
			Except{}.rehashed(p, RehashFound{Rehashed: []mutants.Renewal{{Old: entry, New: now, Line: 6}}})
		}, "rehashed board/board.go:6:11 `>` → `>=` in board#Count (hash ab → cd) in itos-cc.yaml"},
		{"except: the not-rehashed lines", func(p *Presentation) {
			Except{}.rehashed(p, RehashFound{Left: []mutants.StaleException{{Exception: entry, Why: mutants.ExceptionChanged, Line: 6}, {Exception: entry, Why: mutants.ExceptionGone}}})
		}, "  not rehashed board/board.go `>` → `>=` in board#Count (gone)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			tc.do(windows(&stdout, &stderr))
			if !strings.Contains("\n"+stdout.String()+stderr.String(), "\n"+tc.want) {
				t.Errorf("plain output\n%s%s\nwant the line\n%s", stdout.String(), stderr.String(), tc.want)
			}
			if got := stdout.String() + stderr.String(); strings.Contains(got, `\`) {
				t.Errorf("plain output\n%s\nnames a file in the system's form, with a backslash, want it slash-separated", got)
			}
		})
	}
}
