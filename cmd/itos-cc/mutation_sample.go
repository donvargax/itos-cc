package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/donvargax/itos-cc/internal/cli"
	mutation "github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/project"
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
	// Scope is the scope of the tests it ran with now.
	Scope string `json:"scope"`
}

type sampleResult struct {
	Seed  string       `json:"seed"`
	Files []sampleFile `json:"files"`
}

// runMutationSample runs a sample of the chosen functions' cached mutants
// again and compares their outcomes with those recorded.
func runMutationSample(ctx context.Context, in *invocation) (any, error) {
	result := sampleResult{Files: []sampleFile{}}
	count := in.integer("count")
	if count < 1 {
		return result, cli.NewProblem(cli.CodeUsage, "flags.value-invalid", fmt.Sprintf("--count needs a whole number of at least 1, and was given %q", in.str("count")),
			"Give it 1 or more.").With("flag", "--count").With("value", in.str("count"))
	}
	sources, judge, _, _, err := mutationSelection(ctx, in)
	if err != nil {
		return result, err
	}
	if result.Seed, err = sampleSeed(ctx, in); err != nil {
		return result, err
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to sample")
		return result, nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return result, err
	}
	tests, err := importingTests(ctx, in)
	if err != nil {
		return result, err
	}
	support, err := supportNow(ctx, in, cfg)
	if err != nil {
		return result, err
	}
	if in.adapters.Toolchain == nil {
		return result, errors.New("mutation Toolchain binding is missing")
	}
	if in.adapters.Fingerprints == nil {
		return result, errors.New("mutation Fingerprints binding is missing")
	}
	if in.adapters.Workers == nil {
		return nil, errors.New("mutation Workers binding is missing")
	}
	opt := mutation.SampleOptions{
		CheckOptions:  mutationCheckOptions(in, judge, tests, support, nil),
		WorkerFactory: in.adapters.Workers,
		Workers:       in.integer("workers"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Progress:      RunProgress{Log: os.Stderr},
	}
	// An outcome the listed tests decided runs them again, after its own.
	if cfg.Tests != nil && !opt.AllTests && opt.TestCommand == "" {
		opt.Listed = &mutation.Listed{Root: project.Root(), Select: cfg.Tests.Select}
	}
	sampled, err := mutation.Sample(ctx, sources, count, result.Seed, opt)
	if err != nil {
		return result, mutationRunError(err)
	}
	if sampled.Cached == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no cached mutant to sample")
		return result, nil
	}
	if !in.json {
		fmt.Printf("seed %s: sampled %d of %d cached mutants\n", result.Seed, min(count, sampled.Cached), sampled.Cached)
	}
	reported := map[string]bool{}
	for _, f := range sampled.Files {
		if len(f.FailedSelections) > 0 && !f.BaselineFailed {
			result.Files = append(result.Files, sampleFile{File: f.Rel, Baseline: "failed", Mutants: []sampledMutant{}})
			if !in.json {
				fmt.Printf("%s: listed tests its sampled mutants run fail without any mutant; no sampled mutant is judged\n", f.Rel)
			}
			reportFailedSelections(in, f.FailedSelections, reported, "mutation sample")
			continue
		}
		if f.BaselineFailed {
			result.Files = append(result.Files, sampleFile{File: f.Rel, Baseline: "failed", Mutants: []sampledMutant{}})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; no sampled mutant ran\n%s\n", f.Rel, tail(f.BaselineOutput, 20))
			}
			in.report(cli.NewProblem(cli.CodeNo, "mutation.baseline-failed", f.Rel+": its tests fail before any mutant, so none was run",
				"Make its tests pass, then run mutation sample again.").With("file", f.Rel))
			continue
		}
		out := sampleFile{File: f.Rel, Baseline: "passed", Mutants: []sampledMutant{}}
		var differ []mutation.SampledMutant
		for _, m := range f.Mutants {
			out.Mutants = append(out.Mutants, sampledMutant{Line: m.Line, Column: m.Column, Function: m.Function,
				Original: m.Original, Replacement: m.Replacement, Recorded: m.Recorded, Outcome: m.Outcome, Scope: m.Scope})
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
func sampleSeed(ctx context.Context, in *invocation) (string, error) {
	if in.set("seed") {
		return in.str("seed"), nil
	}
	if in.adapters.History == nil {
		return "", fmt.Errorf("mutation history binding is missing")
	}
	id, failure := in.adapters.History.Head(ctx)
	if failure == nil {
		return id, nil
	}
	switch failure := failure.(type) {
	case mutation.NoRepository:
		return "", cli.NewProblem(cli.CodeEnvironment, "sample.no-git", "mutation sample seeds its draw with the HEAD commit's id, so with no --seed it needs a git repository with a commit: "+failure.Reason, "Run it inside a git repository, or give the seed with --seed.")
	case mutation.BadReference:
		return "", failure
	case mutation.HistoryCancelled:
		return "", failure.Cause
	case mutation.HistoryFailed:
		return "", failure.Cause
	}
	return "", failure
}

// reportMismatch lists sampled mutant m of file rel on stdout, as "mismatch
// <file>:<line>:<column> <original> → <replacement> in <function>:
// recorded <outcome>, now <outcome>", and reports it as mutation.mismatch,
// with the scope it ran with and a fix that re-runs it there.
func reportMismatch(in *invocation, rel string, m mutation.SampledMutant) {
	if !in.json {
		fmt.Printf("  mismatch %s:%d:%d %s → %s in %s: recorded %s, now %s\n", rel, m.Line, m.Column,
			quote(m.Original), quote(m.Replacement), m.Function, m.Recorded, m.Outcome)
	}
	p := cli.NewProblem(cli.CodeNo, "mutation.mismatch",
		fmt.Sprintf("%s:%d:%d: %s → %s in %s was recorded %s, and is %s now", rel, m.Line, m.Column,
			quote(m.Original), quote(m.Replacement), m.Function, m.Recorded, m.Outcome),
		fmt.Sprintf("The cached results are not what the tests do: run '%s', and commit .metrics/mutate/ with the code.", rerunIn(m.Scope, rel))).
		With("file", rel).With("line", m.Line).With("column", m.Column).With("function", m.Function).
		With("original", m.Original).With("replacement", m.Replacement).
		With("recorded", m.Recorded).With("outcome", m.Outcome).With("scope", m.Scope)
	p.Shown = true
	in.report(p)
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
	case mutation.ScopeOwn, mutation.ScopeListed, "":
	case mutation.ScopeAllTests:
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
