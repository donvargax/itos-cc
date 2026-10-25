package mutation

import (
	"context"
	"fmt"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type checkFile struct {
	File      string          `json:"file"`
	Functions []checkFunction `json:"functions"`
}

type checkFunction struct {
	Function  string `json:"function"`
	State     string `json:"state"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Excepted  int    `json:"excepted"`
	Uncovered int    `json:"uncovered"`
	// LeftOut is how many mutants its fresh entry records at sites it no
	// longer holds, left out of the counts; left out when there are none.
	LeftOut int `json:"left_out,omitempty"`
}

type checkResult struct {
	Files []checkFile `json:"files"`
	// Orphans are the snapshots under the paths checked whose source file
	// is gone: named, never judged; left out when there are none.
	Orphans []Orphan `json:"orphans,omitempty"`
	// NotBuilt are the Go files selected that only another system builds,
	// whose recorded outcomes are not judged here; left out when there are
	// none.
	NotBuilt []notBuiltFile `json:"not_built,omitempty"`
}

// Run assembles the query once and presents only fully admitted checks.
func (c Check) Run(ctx context.Context, ui *cli.UI, h Handler) int {
	found, err := h.Check(ctx, CheckQuery{SelectionQuery: SelectionQuery{Paths: c.Paths, Changed: c.Changed, Since: c.Since}, FailUncovered: c.FailUncovered != nil, LinesOnly: c.FailUncovered != nil && *c.FailUncovered == "lines"})
	result := checkResult{Files: []checkFile{}}
	for _, orphan := range found.Orphans {
		result.Orphans = append(result.Orphans, Orphan{File: ui.Rel(orphan.Path), Snapshot: orphan.Snapshot, Why: orphan.Why})
	}
	sayExcluded(ui, found.Excluded)
	p := &Presentation{UI: ui, JSON: c.JSON, Root: found.Root, Tree: h.Tree}
	result.NotBuilt = p.sayNotBuilt(found.NotBuilt)
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(readError(err, ui.Rel)))
	}
	if found.NoSources {
		ui.Line("no source files to check")
		p.Elsewhere(ctx, found.Elsewhere)
		p.Orphans(result.Orphans, "is an orphan, which mutation run removes")
		return ui.Emit(c.JSON, result, p.Problems)
	}
	if languages := found.Coverage; c.FailUncovered != nil && len(languages) > 0 {
		for _, coverageCheck := range languages {
			language := coverageCheck.Language
			if !found.Lines.Judges(coverageCheck.File, coverageCheck.Line, coverageCheck.EndLine) {
				continue
			}
			switch coverageCheck.State {
			case "unsupported":
				p.CoverageProblem(language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-unsupported", "uses unsupported "+LanguageName(language)+" coverage scope "+strings.Join(coverageCheck.Changed, ", "))
			case "missing":
				p.CoverageProblem(language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-missing", "has no complete fresh measured "+CoverageInventory(language))
			case "stale":
				p.CoverageProblem(language, coverageCheck.File, coverageCheck.Function, coverageCheck.Line,
					"mutation.coverage-stale", "has "+CoverageEvidenceName(language)+" made before inputs changed: "+strings.Join(coverageCheck.Changed, ", "))
			default:
				for _, block := range coverageCheck.Blocks {
					if !block.Covered && found.Lines.JudgesBlock(coverageCheck.File, block) {
						p.CoverageProblem(language, coverageCheck.File, coverageCheck.Function, block.Line,
							"mutation.uncovered-statement", "has an uncovered "+ExecutableUnit(language))
					}
				}
			}
		}
	}
	for _, file := range found.Files {
		f := checkFile{File: file.Rel, Functions: []checkFunction{}}
		count := map[string]int{}
		for _, fn := range file.Functions {
			count[fn.State]++
			excepted := ExceptedIn(fn.Mutants)
			f.Functions = append(f.Functions, checkFunction{Function: fn.Function, State: fn.State,
				Killed: fn.Entry.Killed, Survived: fn.Entry.Survived - excepted, Excepted: excepted, Uncovered: fn.Entry.Uncovered,
				LeftOut: fn.LeftOut})
		}
		result.Files = append(result.Files, f)
		p.FileChecked(file.Rel, count, file.Functions)
		for _, fn := range file.Functions {
			switch fn.State {
			case mutants.Missing:
				p.Function(file.Rel, fn, "mutation.missing", "has no mutation results")
			case mutants.Stale:
				p.Function(file.Rel, fn, "mutation.stale", staleBecause(fn))
			default:
				p.Failed(file.Rel, fn.Function, fn.Mutants, uncoveredFails(c.FailUncovered != nil, found.Lines, file.Rel))
				p.LeftOut(file.Rel, fn)
			}
		}
		p.StaleExceptions(ctx, file.Rel, file.StaleExceptions)
	}
	p.Elsewhere(ctx, found.Elsewhere)
	p.Orphans(result.Orphans, "is an orphan, which mutation run removes")
	return ui.Emit(c.JSON, result, p.Problems)
}

// FileChecked prints the line of file rel that check judged, "N fresh,
// N stale, N missing in <file>", its file slash-separated after the
// counts of its functions by state, and the test files that made them
// stale.
func (p *Presentation) FileChecked(rel string, count map[string]int, functions []mutants.FunctionCheck) {
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "%d fresh, %d stale, %d missing in %s%s\n", count[mutants.SnapshotFresh], count[mutants.Stale], count[mutants.Missing],
			p.plain(rel), testsNote(functions))
	}
}

func (p *Presentation) LeftOut(rel string, fn mutants.FunctionCheck) {
	if p.JSON || fn.LeftOut == 0 {
		return
	}
	mutants := "mutants"
	if fn.LeftOut == 1 {
		mutants = "mutant"
	}
	_, _ = fmt.Fprintf(p.UI.Stdout, "  left out %s:%d in %s: %d recorded %s at sites gone, which the next mutation run drops\n",
		p.plain(rel), fn.StartLine, fn.Function, fn.LeftOut, mutants)
}

func (p *Presentation) Function(rel string, fn mutants.FunctionCheck, rule, verdict string) {
	what := rule[len("mutation."):]
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "  %s %s:%d in %s\n", what, p.plain(rel), fn.StartLine, fn.Function)
	}
	problem := cli.NewProblem(cli.CodeNo, rule, fmt.Sprintf("%s:%d: %s %s", rel, fn.StartLine, fn.Function, verdict),
		fmt.Sprintf("Run 'itos-cc mutation run %s', and commit .metrics/mutate/ with the code.", rel)).
		With("file", rel).With("line", fn.StartLine).With("function", fn.Function)
	problem.Shown = true
	p.report(problem)
}

func (p *Presentation) Elsewhere(ctx context.Context, stale []mutants.StaleException) {
	for _, e := range stale {
		p.StaleExceptions(ctx, p.UI.Rel(sourcePath(p.Root, e.File)), []mutants.StaleException{e})
	}
}
func (p *Presentation) Orphans(orphans []Orphan, what string) {
	if p.JSON {
		return
	}
	for _, o := range orphans {
		_, _ = fmt.Fprintf(p.UI.Stdout, "snapshot %s %s: its source %s is %s\n", o.Snapshot, what, p.plain(o.File), o.Why)
	}
}
func uncoveredFails(enabled bool, scope mutants.LineScope, path string) func(int) bool {
	if !enabled {
		return nil
	}
	return func(line int) bool { return scope.Judges(path, line, line) }
}
