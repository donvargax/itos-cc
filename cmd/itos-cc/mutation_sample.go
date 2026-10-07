package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

var mutationSampleCommand = &command{
	name:     "mutation sample",
	summary:  "run a sample of the cached mutants again: do their outcomes hold?",
	synopsis: "[options] [path ...]",
	about: `
Runs again a few of the mutants whose results mutation run cached in
.metrics/mutate/, and fails when an outcome differs from the one recorded,
so CI catches a cache written by hand, or against other code, which
mutation check trusts. It writes nothing: no snapshot and no summary
comment.

It draws from the fresh entries of the functions chosen, fresh as mutation
check judges them, and among them only from the mutants that ran: killed,
timeout, or survived, never uncovered, so no coverage command runs. Stale
and missing results are mutation check's to report. Each test command's
baseline runs first, then each sampled mutant, in a worker's copy of the
project, as in mutation run.

--count N draws N mutants from the whole selection, all of them when there
are fewer. The draw is seeded with the HEAD commit's id, so a rerun at one
commit samples the same mutants and each new commit others; --seed TEXT
replaces it, to reproduce a run from the seed it printed. Outside a git
repository, --seed is needed.

An outcome that differs from the recorded one fails, whichever way it went:
a kill that now survives, or a survivor now killed. Killed and timeout
agree, since both mean the tests noticed. A recorded survivor that still
survives is no failure here: failing on survivors is mutation check's.

It chooses as mutation check does: paths, --changed, and --since REF, which
samples only the functions the commits since REF changed (git diff
REF...HEAD). Run it with the --all-tests and --test-command the results were
recorded with, or kills made by other tests read as survivors.

Plain output is the seed and how many of the cached mutants were sampled,
then a line per file, "<file>: N sampled, N mismatched", then each mismatch:
  mismatch <file>:<line>:<column> ` + "`original` → `replacement`" + ` in <namespace#name>: recorded <outcome>, now <outcome>
With --json, each file's "mutants" holds the mutants sampled, in site order;
it is empty when the file's baseline failed.`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		opt("since", stringFlag, "REF", "", "sample only the functions the commits since REF changed (git diff REF...HEAD)"),
		opt("count", intFlag, "N", "20", "how many mutants to run, at least 1"),
		opt("seed", stringFlag, "TEXT", "", "seed the draw with TEXT instead of the HEAD commit's id"),
		opt("workers", intFlag, "N", fmt.Sprint(max(1, runtime.NumCPU()/2)), "mutants run at the same time"),
		opt("timeout-factor", floatFlag, "N", "10", "a mutant times out after N times the baseline duration, and at least 2s"),
		opt("test-command", stringFlag, "CMD", "", "shell command that runs the tests, instead of the per-language default"),
		sw("all-tests", "run the whole test suite for every mutant, integration and end-to-end tests included")),
	json: `"seed", "files": [{"file", "baseline": "passed"|"failed",
   "mutants": [{"line", "column", "function", "original", "replacement",
   "recorded", "outcome": "killed"|"survived"|"timeout"}]}]`,
	rules: []string{
		"mutation.mismatch         a sampled mutant's outcome differs from the one recorded: file, line, column, function, original, replacement, recorded, outcome",
		"mutation.baseline-failed  the tests fail before any mutant: file",
		"sample.no-git             no --seed outside a git repository",
		"since.bad-ref             --since names no commit: ref",
		"since.no-git              --since outside a git repository",
		"flags.conflict            --since with --changed: flag",
	},
	exits: []exitDoc{
		{0, "every sampled mutant's outcome agrees with the one recorded, or there was nothing to sample"},
		{1, "a sampled mutant's outcome differs from the one recorded, or a file's tests fail before any mutant"},
		{2, "a usage error: a bad flag or path, a --count below 1, a --since ref that is no commit, or --since with --changed"},
		{3, "outside a git repository: with no --seed, or with --changed or --since"},
	},
	examples: []string{
		"itos-cc mutation sample                        # in CI: 20 mutants, seeded with HEAD",
		"itos-cc mutation sample --since origin/main --count 100",
		"itos-cc mutation sample --seed 4813e48 --json  # reproduce a run",
	},
	run: runMutationSample,
}

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
}

type sampleResult struct {
	Seed  string       `json:"seed"`
	Files []sampleFile `json:"files"`
}

// runMutationSample runs a sample of the chosen functions' cached mutants
// again and compares their outcomes with those recorded.
func runMutationSample(in *invocation) (any, error) {
	result := sampleResult{Files: []sampleFile{}}
	count := in.integer("count")
	if count < 1 {
		return result, fail(kindUsage, "flags.value-invalid", fmt.Sprintf("--count needs a whole number of at least 1, and was given %q", in.str("count")),
			"Give it 1 or more.").with("flag", "--count").with("value", in.str("count"))
	}
	sources, judge, _, err := mutationSelection(in)
	if err != nil {
		return result, err
	}
	if result.Seed, err = sampleSeed(in); err != nil {
		return result, err
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to sample")
		return result, nil
	}
	tests, err := importingTests()
	if err != nil {
		return result, err
	}
	sampled, err := mutate.Sample(sources, count, result.Seed, mutate.Options{
		Tests:         tests,
		Judge:         judge,
		Workers:       in.integer("workers"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Log:           os.Stderr,
	})
	if err != nil {
		return result, err
	}
	if sampled.Cached == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no cached mutant to sample")
		return result, nil
	}
	if !in.json {
		fmt.Printf("seed %s: sampled %d of %d cached mutants\n", result.Seed, min(count, sampled.Cached), sampled.Cached)
	}
	for _, f := range sampled.Files {
		if f.BaselineFailed {
			result.Files = append(result.Files, sampleFile{File: f.Rel, Baseline: "failed", Mutants: []sampledMutant{}})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; no sampled mutant ran\n%s\n", f.Rel, tail(f.BaselineOutput, 20))
			}
			in.report(fail(kindNo, "mutation.baseline-failed", f.Rel+": its tests fail before any mutant, so none was run",
				"Make its tests pass, then run mutation sample again.").with("file", f.Rel))
			continue
		}
		out := sampleFile{File: f.Rel, Baseline: "passed", Mutants: []sampledMutant{}}
		var differ []mutate.SampledMutant
		for _, m := range f.Mutants {
			out.Mutants = append(out.Mutants, sampledMutant{Line: m.Line, Column: m.Column, Function: m.Function,
				Original: m.Original, Replacement: m.Replacement, Recorded: m.Recorded, Outcome: m.Outcome})
			if !m.Agrees() {
				differ = append(differ, m)
			}
		}
		result.Files = append(result.Files, out)
		if !in.json {
			fmt.Printf("%s: %d sampled, %d mismatched\n", f.Rel, len(f.Mutants), len(differ))
		}
		for _, m := range differ {
			reportMismatch(in, f.Rel, m)
		}
	}
	return result, nil
}

// sampleSeed is --seed, or the HEAD commit's id.
func sampleSeed(in *invocation) (string, error) {
	if in.set("seed") {
		return in.str("seed"), nil
	}
	id, err := project.Head()
	var noGit *project.NoGitError
	if errors.As(err, &noGit) {
		return "", fail(kindMissing, "sample.no-git", "mutation sample seeds its draw with the HEAD commit's id, so with no --seed it needs a git repository with a commit: "+noGit.Reason,
			"Run it inside a git repository, or give the seed with --seed.")
	}
	return id, err
}

// reportMismatch lists sampled mutant m of file rel on stdout, as "mismatch
// <file>:<line>:<column> <original> → <replacement> in <function>:
// recorded <outcome>, now <outcome>", and reports it as mutation.mismatch.
func reportMismatch(in *invocation, rel string, m mutate.SampledMutant) {
	if !in.json {
		fmt.Printf("  mismatch %s:%d:%d %s → %s in %s: recorded %s, now %s\n", rel, m.Line, m.Column,
			quote(m.Original), quote(m.Replacement), m.Function, m.Recorded, m.Outcome)
	}
	p := fail(kindNo, "mutation.mismatch",
		fmt.Sprintf("%s:%d:%d: %s → %s in %s was recorded %s, and is %s now", rel, m.Line, m.Column,
			quote(m.Original), quote(m.Replacement), m.Function, m.Recorded, m.Outcome),
		fmt.Sprintf("The cached results are not what the tests do: run 'itos-cc mutation run --mutate-all %s', and commit .metrics/mutate/ with the code.", rel)).
		with("file", rel).with("line", m.Line).with("column", m.Column).with("function", m.Function).
		with("original", m.Original).with("replacement", m.Replacement).
		with("recorded", m.Recorded).with("outcome", m.Outcome)
	p.shown = true
	in.report(p)
}
