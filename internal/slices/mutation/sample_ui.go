package mutation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type sampleFile struct {
	File     string          `json:"file"`
	Baseline string          `json:"baseline"`
	Mutants  []sampledMutant `json:"mutants"`
}

type sampledMutant struct {
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Recorded    string `json:"recorded"`
	Outcome     string `json:"outcome"`
	// Outcomes is every outcome its runs gave, in run order: more than
	// one when it disagreed with the one recorded and ran again.
	Outcomes []string `json:"outcomes"`
	// Scope is the scope of the tests it ran with now.
	Scope string `json:"scope"`
}

type sampleResult struct {
	Seed  string       `json:"seed"`
	Files []sampleFile `json:"files"`
	// NotBuilt are the Go files selected that only another system builds,
	// none of whose recorded mutants is drawn; left out when there are
	// none.
	NotBuilt []notBuiltFile `json:"not_built,omitempty"`
}

// Written is the flags the validated command line gave, by name, each with
// its values as written (cli.Provided), which cmd binds as cli.Written and
// run and sample convert: a refusal quotes a value back as the person wrote
// it.
type Written map[string][]string

// spelling is the last value the command line gave flag name, as written,
// or value when it gave none.
func (w Written) spelling(name, value string) string {
	if given := w[name]; len(given) > 0 {
		return given[len(given)-1]
	}
	return value
}

// selectionProgress shows the observations of sample and run as they
// happen on stderr: the sources exclude left out, then the run's progress.
type selectionProgress struct {
	Progress
	ui *cli.UI
}

func (p selectionProgress) Excluded(n int) { sayExcluded(p.ui, n) }

// Run refuses a --count below 1 before anything is observed, assembles the
// query once, then presents the sample, or the result object as far as it
// got beside the failure; the missing Workers binding alone gives none.
func (c Sample) Run(ctx context.Context, ui *cli.UI, h Handler, given cli.Written) int {
	written := Written(given)
	result := sampleResult{Files: []sampleFile{}}
	if c.Count < 1 {
		value := written.spelling("count", strconv.Itoa(c.Count))
		problem := cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--count needs a whole number of at least 1, and was given %q", value),
			"Give it 1 or more.").With("flag", "--count").With("value", value)
		return ui.Emit(c.JSON, result, ui.Problems(problem))
	}
	found, err := h.Sample(ctx, SampleQuery{
		SelectionQuery: SelectionQuery{Paths: c.Paths, Changed: c.Changed, Since: c.Since},
		Count:          c.Count, Seed: c.Seed, Workers: c.Workers, TimeoutFactor: c.TimeoutFactor,
		OutputFactor: c.OutputFactor, MemoryFactor: c.MemoryFactor,
		TestCommand: c.TestCommand, AllTests: c.AllTests,
		Progress: selectionProgress{Progress: Progress{Log: ui.Stderr, Rel: ui.Rel}, ui: ui},
	})
	result.Seed = found.Seed
	result.NotBuilt = sayNotBuilt(ui, found.NotBuilt)
	if errors.Is(err, errNoWorkers) {
		return ui.Emit(c.JSON, nil, ui.Problems(err))
	}
	if err != nil {
		return ui.Emit(c.JSON, result, ui.Problems(sampleError(err, ui.Rel)))
	}
	if found.NoSources {
		ui.Line("no source files to sample")
		return ui.Emit(c.JSON, result, nil)
	}
	sampled := found.Sampled
	if sampled.Cached == 0 {
		ui.Line("no cached mutant to sample")
		return ui.Emit(c.JSON, result, nil)
	}
	if !c.JSON {
		_, _ = fmt.Fprintf(ui.Stdout, "seed %s: sampled %d of %d cached mutants\n", result.Seed, min(c.Count, sampled.Cached), sampled.Cached)
	}
	p := &Presentation{UI: ui, JSON: c.JSON}
	reported := map[string]bool{}
	for _, f := range sampled.Files {
		p.sampledFile(f, &result, reported)
	}
	return ui.Emit(c.JSON, result, p.Problems)
}

// sampledFile presents file f of a sample, adding it to result: a failed
// selection of listed tests or baseline, which judges none of its sampled
// mutants, or how many were sampled and each that mismatched. reported
// holds the selections reported so far.
func (p *Presentation) sampledFile(f mutants.SampledFile, result *sampleResult, reported map[string]bool) {
	if len(f.FailedSelections) > 0 && !f.BaselineFailed {
		result.Files = append(result.Files, sampleFile{File: f.Rel, Baseline: "failed", Mutants: []sampledMutant{}})
		if !p.JSON {
			_, _ = fmt.Fprintf(p.UI.Stdout, "listed tests the sampled mutants of %s run fail without any mutant; no sampled mutant is judged\n", f.Rel)
		}
		p.FailedSelections(f.FailedSelections, reported, "mutation sample")
		return
	}
	if f.BaselineFailed {
		result.Files = append(result.Files, sampleFile{File: f.Rel, Baseline: "failed", Mutants: []sampledMutant{}})
		if !p.JSON {
			_, _ = fmt.Fprintf(p.UI.Stdout, "baseline tests of %s fail; no sampled mutant ran\n%s\n", f.Rel, HeadAndTail(f.BaselineOutput, reportedLines))
		}
		p.report(p.baselineFailed(f.Rel, f.BaselineCopy, f.BaselineLeftOut, "run", "mutation sample").With("output", HeadAndTail(f.BaselineOutput, reportedLines)))
		return
	}
	out := sampleFile{File: f.Rel, Baseline: "passed", Mutants: []sampledMutant{}}
	var differ []mutants.SampledMutant
	verdicts := map[string]int{}
	for _, m := range f.Mutants {
		out.Mutants = append(out.Mutants, sampledMutant{Line: m.Line, Column: m.Column, Function: m.Function,
			Original: m.Original, Replacement: m.Replacement, Recorded: m.Recorded, Outcome: m.Outcome, Outcomes: m.Outcomes, Scope: m.Scope})
		if verdict := m.Verdict(); verdict != mutants.Agreed {
			differ = append(differ, m)
			verdicts[verdict]++
		}
	}
	result.Files = append(result.Files, out)
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "%d sampled, %d mismatched, %d flaky in %s\n", len(f.Mutants), verdicts[mutants.Mismatched], verdicts[mutants.Flaky], f.Rel)
	}
	for _, m := range differ {
		if m.Verdict() == mutants.Flaky {
			p.Flaky(f.Rel, m)
			continue
		}
		p.Mismatch(f.Rel, m)
	}
}

// Flaky lists sampled mutant m of file rel on stdout, as "flaky
// <original> → <replacement> in <function>, <file> line <line> column
// <column>: recorded <outcome>, now <outcome>, <outcome>", every outcome
// its runs gave in run order, and reports it as mutation.flaky, with the
// fields of a mismatch and "outcomes": its tests decided it differently
// with nothing changed, so the fix is in them, not in a mutation run.
func (p *Presentation) Flaky(rel string, m mutants.SampledMutant) {
	now := strings.Join(m.Outcomes, ", ")
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "  flaky %s → %s in %s, %s: recorded %s, now %s\n",
			Quote(m.Original), Quote(m.Replacement), m.Function, SiteAt(rel, m.Line, m.Column), m.Recorded, now)
	}
	problem := cli.NewProblem(cli.CodeNo, "mutation.flaky",
		fmt.Sprintf("%s:%d:%d: %s → %s in %s was recorded %s, and its runs now gave %s: its tests decide it differently with nothing changed",
			rel, m.Line, m.Column, Quote(m.Original), Quote(m.Replacement), m.Function, m.Recorded, now),
		"Make its tests deterministic, seeding their randomness and settling their timing, and commit them.").
		With("file", rel).With("line", m.Line).With("column", m.Column).With("function", m.Function).
		With("original", m.Original).With("replacement", m.Replacement).
		With("recorded", m.Recorded).With("outcome", m.Outcome).With("outcomes", m.Outcomes).With("scope", m.Scope)
	problem.Shown = true
	p.report(problem)
}

// Mismatch lists sampled mutant m of file rel on stdout, as "mismatch
// <original> → <replacement> in <function>, <file> line <line> column
// <column>: recorded <outcome>, now <outcome>", and reports it as mutation.mismatch,
// with the scope it ran with and a fix that re-runs it there, its command
// last, after a colon and in no quotes of its own, so it pastes as written
// whatever quotes a --test-command line brings (slice-45).
func (p *Presentation) Mismatch(rel string, m mutants.SampledMutant) {
	if !p.JSON {
		_, _ = fmt.Fprintf(p.UI.Stdout, "  mismatch %s → %s in %s, %s: recorded %s, now %s\n",
			Quote(m.Original), Quote(m.Replacement), m.Function, SiteAt(rel, m.Line, m.Column), m.Recorded, m.Outcome)
	}
	problem := cli.NewProblem(cli.CodeNo, "mutation.mismatch",
		fmt.Sprintf("%s:%d:%d: %s → %s in %s was recorded %s, and is %s now", rel, m.Line, m.Column,
			Quote(m.Original), Quote(m.Replacement), m.Function, m.Recorded, m.Outcome),
		"The cached results are not what the tests do: commit .metrics/mutate/ with the code after running: "+rerunIn(m.Scope, rel)).
		With("file", rel).With("line", m.Line).With("column", m.Column).With("function", m.Function).
		With("original", m.Original).With("replacement", m.Replacement).
		With("recorded", m.Recorded).With("outcome", m.Outcome).With("scope", m.Scope)
	problem.Shown = true
	p.report(problem)
}

// rerunIn is the mutation run that re-records the mutants of file rel in
// scope, the scope a sampled mutant ran with: following a mismatch's fix
// in another scope would record its outcome under other tests, and a kill
// only the whole suite makes would turn into a survivor. "own" and
// "listed" need no flag, since a run without one uses the file's own tests,
// then the listed tests; "all-tests" is --all-tests, and any other scope is
// the --test-command line, single-quoted for a POSIX shell.
func rerunIn(scope, rel string) string {
	flags := ""
	switch scope {
	case mutants.ScopeOwn, mutants.ScopeListed, "":
	case mutants.ScopeAllTests:
		flags = "--all-tests "
	default:
		flags = "--test-command " + shellQuote(scope) + " "
	}
	return "itos-cc mutation run --mutate-all " + flags + rel
}

// shellQuote is s in single quotes, for a POSIX shell: each single quote
// in it ends the quoting, is escaped with a backslash, and starts it again.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
