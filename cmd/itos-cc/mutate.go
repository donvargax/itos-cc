package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
	"github.com/donvargax/itos-cc/project"
)

const (
	exitBaseline = 2
	exitSurvived = 3
)

func runMutate(args []string) int {
	fs := flag.NewFlagSet("mutate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: itos-cc mutate [options] [path ...]

Changes one operator, boolean, or 0/1 at a time inside each function and runs
the tests that cover the file. A mutant is killed when the tests fail or time
out and survives when they pass. Mutants on lines no test executes are
uncovered and are not run.

Results are cached in .metrics/mutate/<file>.json, which is meant to be
committed: later runs reuse killed mutants of unchanged functions and retry
only survivors and changed functions. A summary comment is kept at the end of
each source file.

The first run of a tree executes every covered mutant; start with the file
you are working on. Exits 2 when a baseline test run fails and 3 when a
mutant survives.

`)
		fs.PrintDefaults()
	}
	var sel selection
	var cov coverageOptions
	sel.register(fs)
	cov.register(fs)
	opt := mutate.Options{Annotate: true, Log: os.Stderr}
	fs.IntVar(&opt.Workers, "workers", max(1, runtime.NumCPU()/2), "mutants run at the same time")
	fs.BoolVar(&opt.MutateAll, "mutate-all", false, "rerun killed mutants of unchanged functions too")
	fs.Float64Var(&opt.TimeoutFactor, "timeout-factor", 10, "a mutant times out after this many times the baseline duration (at least 2s)")
	fs.StringVar(&opt.TestCommand, "test-command", "", "shell command that runs the tests, instead of the per-language default")
	noAnnotate := fs.Bool("no-annotate", false, "do not write the summary comment into source files")
	scan := fs.Bool("scan", false, "list mutation sites without running tests")
	paths, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	opt.Annotate = !*noAnnotate

	files, err := sel.files(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	if len(files.Sources) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no source files to mutate")
		return exitOK
	}
	if *scan {
		return scanSites(files.Sources)
	}
	if !cov.none {
		opt.Coverage = func(sources []string) *coverage.Report {
			report, err := cov.load(sources, os.Stderr)
			if err != nil {
				fmt.Fprintln(os.Stderr, "itos-cc: coverage:", err)
				return nil
			}
			return report
		}
	}

	results, err := mutate.Run(files.Sources, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc:", err)
		return exitUsage
	}
	code := exitOK
	for _, r := range results {
		if r.BaselineFailed {
			fmt.Printf("%s: baseline tests fail; snapshot not updated\n%s\n", r.Rel, tail(r.BaselineOutput, 20))
			code = exitBaseline
			continue
		}
		var killed, survived, uncovered int
		for _, u := range r.Snapshot.Units {
			killed += u.Killed
			survived += u.Survived
			uncovered += u.Uncovered
		}
		fmt.Printf("%s: %d killed, %d survived, %d uncovered (ran %d, reused %d)\n",
			r.Rel, killed, survived, uncovered, r.Ran, r.Reused)
		for _, u := range r.Snapshot.Units {
			for _, m := range u.Mutants {
				if m.Outcome == mutate.Survived {
					fmt.Printf("  survived %s:%d:%d %s → %s in %s#%s\n", r.Rel, m.Line, m.Column,
						quote(m.Original), quote(m.Replacement), u.Namespace, u.Name)
				}
			}
		}
		if survived > 0 && code == exitOK {
			code = exitSurvived
		}
	}
	return code
}

func scanSites(sources []string) int {
	for _, path := range sources {
		f, err := lang.ParseFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "itos-cc:", err)
			return exitUsage
		}
		for _, s := range mutate.Sites(f) {
			u := f.Units[s.Unit]
			fmt.Printf("%s:%d:%d %s → %s in %s#%s\n", project.Rel(path), s.Line, s.Column,
				quote(s.Original), quote(s.Replacement), u.Namespace, u.Name)
		}
		f.Close()
	}
	return exitOK
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
