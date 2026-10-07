package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

var mutateCommand = &command{
	name:     "mutate",
	summary:  "mutation testing: do the tests notice small changes?",
	synopsis: "[options] [path ...]",
	about: `
Changes one operator, boolean, or 0/1 at a time inside each function and runs
the file's own tests: its Go package, the Vitest or Jest tests that import it,
or the whole suite where nothing narrower exists. A mutant is killed when the
tests fail or time out and survives when they pass. Mutants on lines those
tests never execute are uncovered and are not run.

--all-tests runs the whole suite for coverage and for every mutant, so
integration and end-to-end tests can kill mutants too. It is slow: run it
nightly. Add --no-coverage for end-to-end tests that only run the built
binary, which coverage does not see.

Results are cached in .metrics/mutate/<file>.json, which is meant to be
committed: later runs reuse killed mutants of unchanged functions and retry
only survivors and changed functions. A summary comment is kept at the end of
each source file.`,
	flags: append(append(append([]flagSpec{}, selectionFlags...), coverageFlags...),
		opt("workers", intFlag, "N", fmt.Sprint(max(1, runtime.NumCPU()/2)), "mutants run at the same time"),
		sw("mutate-all", "rerun killed mutants of unchanged functions too"),
		opt("timeout-factor", floatFlag, "N", "10", "a mutant times out after N times the baseline duration, and at least 2s"),
		opt("test-command", stringFlag, "CMD", "", "shell command that runs the tests, instead of the per-language default"),
		sw("no-annotate", "do not write the summary comment into source files"),
		sw("scan", "list mutation sites without running tests"),
		opt("since", stringFlag, "REF", "", "judge only the functions the commits since REF changed")),
	json: `"files": [{"file", "killed", "survived", "uncovered", "ran", "reused",
   "baseline": "passed"|"failed"}]; with --scan, "sites": [{"file", "line",
   "column", "function", "original", "replacement"}]`,
	rules: []string{
		"mutate.survived         a mutant survived: file, line, column, function, original, replacement",
		"mutate.baseline-failed  the tests fail before any mutant: file",
	},
	exits: []exitDoc{
		{0, "every mutant that ran was killed"},
		{1, "a mutant survived, or a file's tests fail before any mutant"},
		{2, "a usage error: a bad flag or path"},
		{3, "--changed outside a git repository"},
	},
	examples: []string{
		"itos-cc mutate --changed",
		"itos-cc mutate --all-tests --json        # nightly",
	},
	run: runMutate,
}

type mutateFile struct {
	File      string `json:"file"`
	Killed    int    `json:"killed"`
	Survived  int    `json:"survived"`
	Uncovered int    `json:"uncovered"`
	Ran       int    `json:"ran"`
	Reused    int    `json:"reused"`
	Baseline  string `json:"baseline"`
}

type mutateResult struct {
	Files []mutateFile `json:"files"`
}

type mutateSite struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}

func runMutate(in *invocation) (any, error) {
	result := mutateResult{Files: []mutateFile{}}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to mutate")
		return result, nil
	}
	if in.set("scan") {
		return scanSites(in, files.Sources)
	}
	opt := mutate.Options{
		Workers:       in.integer("workers"),
		MutateAll:     in.set("mutate-all"),
		TimeoutFactor: in.float("timeout-factor"),
		TestCommand:   in.str("test-command"),
		AllTests:      in.set("all-tests"),
		Annotate:      !in.set("no-annotate"),
		Log:           os.Stderr,
	}
	if !in.set("no-coverage") {
		opt.Coverage = func(sources []string) *coverage.Report {
			// Coverage comes from the tests that kill mutants, so a line only
			// other tests reach is uncovered rather than a survivor.
			report, err := loadCoverage(in, sources, coverage.OwnTests, os.Stderr)
			if err != nil {
				fmt.Fprintln(os.Stderr, "itos-cc: coverage:", err)
				return nil
			}
			return report
		}
	}

	results, err := mutate.Run(files.Sources, opt)
	if err != nil {
		return result, err
	}
	for _, r := range results {
		if r.BaselineFailed {
			result.Files = append(result.Files, mutateFile{File: r.Rel, Baseline: "failed"})
			if !in.json {
				fmt.Printf("%s: baseline tests fail; snapshot not updated\n%s\n", r.Rel, tail(r.BaselineOutput, 20))
			}
			in.report(fail(kindNo, "mutate.baseline-failed", r.Rel+": its tests fail before any mutant, so none was judged",
				"Make its tests pass, then run mutate again.").with("file", r.Rel))
			continue
		}
		f := mutateFile{File: r.Rel, Ran: r.Ran, Reused: r.Reused, Baseline: "passed"}
		for _, u := range r.Snapshot.Units {
			f.Killed += u.Killed
			f.Survived += u.Survived
			f.Uncovered += u.Uncovered
		}
		result.Files = append(result.Files, f)
		if !in.json {
			fmt.Printf("%s: %d killed, %d survived, %d uncovered (ran %d, reused %d)\n",
				r.Rel, f.Killed, f.Survived, f.Uncovered, f.Ran, f.Reused)
		}
		for _, u := range r.Snapshot.Units {
			for _, m := range u.Mutants {
				if m.Outcome != mutate.Survived {
					continue
				}
				function := u.Namespace + "#" + u.Name
				if !in.json {
					fmt.Printf("  survived %s:%d:%d %s → %s in %s\n", r.Rel, m.Line, m.Column,
						quote(m.Original), quote(m.Replacement), function)
				}
				p := fail(kindNo, "mutate.survived",
					fmt.Sprintf("%s:%d:%d: %s → %s in %s survived", r.Rel, m.Line, m.Column, quote(m.Original), quote(m.Replacement), function),
					"Add a test that fails with this change.").
					with("file", r.Rel).with("line", m.Line).with("column", m.Column).with("function", function).
					with("original", m.Original).with("replacement", m.Replacement)
				p.shown = true
				in.report(p)
			}
		}
	}
	return result, nil
}

func scanSites(in *invocation, sources []string) (any, error) {
	sites := []mutateSite{}
	for _, path := range sources {
		f, err := lang.ParseFile(path)
		if err != nil {
			return nil, err
		}
		for _, s := range mutate.Sites(f) {
			u := f.Units[s.Unit]
			site := mutateSite{File: project.Rel(path), Line: s.Line, Column: s.Column,
				Function: u.Namespace + "#" + u.Name, Original: s.Original, Replacement: s.Replacement}
			sites = append(sites, site)
			if !in.json {
				fmt.Printf("%s:%d:%d %s → %s in %s\n", site.File, site.Line, site.Column,
					quote(site.Original), quote(site.Replacement), site.Function)
			}
		}
		f.Close()
	}
	return struct {
		Sites []mutateSite `json:"sites"`
	}{sites}, nil
}

func quote(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}
