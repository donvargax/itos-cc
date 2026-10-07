package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// Exit codes shared by every command.
const (
	exitOK        = 0
	exitUsage     = 1
	exitThreshold = 2
)

// parse parses flags anywhere among the arguments, so
// `itos-cc crap src --json` works as well as `itos-cc crap --json src`.
// Asked for help, it prints the command's usage to stdout and returns
// flag.ErrHelp.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-h" || a == "-help" || a == "--help" {
			fs.SetOutput(os.Stdout)
		}
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// parseExit is the exit code for a parse error: help was asked for and
// given, or the arguments were misused.
func parseExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}

// selection is how a command chooses its files.
type selection struct {
	changed bool
}

func (s *selection) register(fs *flag.FlagSet) {
	fs.BoolVar(&s.changed, "changed", false, "only files git reports as added or modified")
}

// files resolves the command's paths. A path that does not exist is a
// fragment: every source under the working directory whose path contains it
// is selected, so `itos-cc crap billing` finds src/billing/*.
func (s *selection) files(paths []string) (project.Files, error) {
	if s.changed {
		changed, err := project.Changed()
		if err != nil {
			return project.Files{}, fmt.Errorf("--changed needs a git repository: %w", err)
		}
		if len(changed) == 0 {
			return project.Files{}, nil
		}
		paths = append(paths, changed...)
	}
	if len(paths) == 0 {
		return project.Discover([]string{"."})
	}
	var roots, fragments []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
		} else {
			fragments = append(fragments, filepath.ToSlash(p))
		}
	}
	files, err := project.Discover(roots)
	if err != nil || len(fragments) == 0 {
		return files, err
	}
	all, err := project.Discover([]string{"."})
	if err != nil {
		return files, err
	}
	keep := func(list []string) []string {
		var out []string
		for _, f := range list {
			for _, frag := range fragments {
				if strings.Contains(filepath.ToSlash(f), frag) {
					out = append(out, f)
					break
				}
			}
		}
		return out
	}
	files.Sources = append(files.Sources, keep(all.Sources)...)
	files.Tests = append(files.Tests, keep(all.Tests)...)
	return files, nil
}

// coverageOptions is how a command gets coverage.
type coverageOptions struct {
	none     bool
	existing bool
	allTests bool
	command  string
	reports  stringList
	// focused is set when paths or --changed chose the files, so only the
	// tests that load them need to run.
	focused bool
}

func (c *coverageOptions) register(fs *flag.FlagSet) {
	fs.BoolVar(&c.none, "no-coverage", false, "skip coverage; complexity only")
	fs.BoolVar(&c.existing, "use-existing-coverage", false, "read reports already on disk instead of running tests")
	fs.BoolVar(&c.allTests, "all-tests", false, "run the whole test suite, not only the tests that load the chosen files")
	fs.StringVar(&c.command, "coverage-command", "", "run this shell command instead of the default per-language commands (use with --coverage-report)")
	fs.Var(&c.reports, "coverage-report", "read this LCOV, Go cover profile, or JaCoCo XML report (repeatable)")
}

// load produces coverage for sources, or nil with --no-coverage. Progress and
// test output go to log so stdout stays the report.
func (c *coverageOptions) load(sources []string, log io.Writer) (*coverage.Report, error) {
	switch {
	case c.none:
		return nil, nil
	case c.command != "":
		if len(c.reports) == 0 {
			return nil, fmt.Errorf("--coverage-command needs --coverage-report to say where the report lands")
		}
		fmt.Fprintf(log, "coverage: $ %s\n", c.command)
		if err := shell(c.command, log); err != nil {
			fmt.Fprintf(log, "coverage: %v\n", err)
		}
		return coverage.Files(c.reports, sources, log), nil
	case len(c.reports) > 0:
		return coverage.Files(c.reports, sources, log), nil
	}
	out := filepath.Join(metrics.Dir, "coverage")
	plans := coverage.Plans(sources, absOrSame(out), c.allTests || !c.focused)
	if c.existing {
		return coverage.Existing(plans, sources, log), nil
	}
	if err := coverage.IgnoreDir(out); err != nil {
		return nil, err
	}
	return coverage.Run(plans, sources, log), nil
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func absOrSame(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
