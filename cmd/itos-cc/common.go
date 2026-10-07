package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// selectionFlags choose a command's files.
var selectionFlags = []flagSpec{sw("changed", "only files git reports as added or modified")}

// files resolves the command's paths. A path that does not exist is a
// fragment: every file under the working directory whose path contains it
// is selected, so `itos-cc crap billing` finds src/billing/*. An argument
// that is neither a path nor a fragment of one is refused.
func files(in *invocation) (project.Files, error) {
	paths := in.args
	if in.set("changed") {
		changed, err := project.Changed()
		if err != nil {
			return project.Files{}, fail(kindMissing, "changed.no-git", "--changed needs a git repository: "+err.Error(),
				"Run it inside a git repository, or name the paths instead.")
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
	matched := map[string]bool{}
	keep := func(list []string) []string {
		var out []string
		for _, f := range list {
			for _, frag := range fragments {
				if strings.Contains(filepath.ToSlash(f), frag) {
					out = append(out, f)
					matched[frag] = true
					break
				}
			}
		}
		return out
	}
	files.Sources = append(files.Sources, keep(all.Sources)...)
	files.Tests = append(files.Tests, keep(all.Tests)...)
	for _, frag := range fragments {
		if !matched[frag] {
			return files, fail(kindUsage, "paths.unmatched", fmt.Sprintf("%q is no file, and no file's path under the working directory contains it", frag),
				"Name a file, a directory, or a fragment of a source's path.").with("argument", frag)
		}
	}
	return files, nil
}

// coverageFlags say how a command gets coverage.
var coverageFlags = []flagSpec{
	sw("no-coverage", "skip coverage; complexity only"),
	sw("use-existing-coverage", "read reports already on disk instead of running tests"),
	sw("all-tests", "run the whole test suite, integration and end-to-end tests included"),
	opt("coverage-command", stringFlag, "CMD", "", "run this shell command instead of the per-language commands (with --coverage-report)"),
	{name: "coverage-report", typ: stringFlag, arg: "FILE", repeat: true, help: "read this LCOV, Go cover profile, or JaCoCo XML report"},
}

// loadCoverage produces coverage for sources, or nil with --no-coverage.
// scope is the tests that run without --all-tests. With perTest, when the
// coverage commands run, it also measures each listed test's coverage, in
// the run's own directory. Progress and test output go to log so stdout
// stays the report.
func loadCoverage(in *invocation, sources []string, scope coverage.Scope, log io.Writer, perTest *coverage.PerTest) (*coverage.Report, error) {
	command, reports := in.str("coverage-command"), in.strs("coverage-report")
	switch {
	case in.set("no-coverage"):
		return nil, nil
	case command != "":
		if len(reports) == 0 {
			return nil, fail(kindUsage, "coverage.command-needs-report", "--coverage-command needs --coverage-report to say where its report lands",
				"Add --coverage-report FILE.")
		}
		fmt.Fprintf(log, "coverage: $ %s\n", command)
		if err := shell(command, log); err != nil {
			fmt.Fprintf(log, "coverage: %v\n", err)
		}
		report := coverage.Files(reports, sources, log)
		// The command was to write the report, so a report it did not
		// write means it measured nothing.
		missing := report.Missing()
		for i := range missing {
			if missing[i].Cause == coverage.Unreadable {
				missing[i].Cause, missing[i].Reason = coverage.MeasuredNothing, "--coverage-command wrote no readable report: "+missing[i].Reason
			}
		}
		return report, nil
	case len(reports) > 0:
		return coverage.Files(reports, sources, log), nil
	}
	out := filepath.Join(metrics.Dir(), "coverage")
	if in.set("all-tests") {
		scope = coverage.AllTests
	}
	if in.set("use-existing-coverage") {
		return coverage.Existing(coverage.Plans(sources, out, scope), sources, log), nil
	}
	if err := coverage.IgnoreDir(out); err != nil {
		return nil, err
	}
	// Each run writes its reports in a directory of its own, so runs at the
	// same time, such as an agent's and a commit hook's, never read each
	// other's, then keeps only the latest for --use-existing-coverage.
	removeAbandoned(out)
	run, err := os.MkdirTemp(out, "run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(run)
	report := coverage.Run(coverage.Plans(sources, run, scope), sources, log)
	if perTest != nil {
		report.SetTests(coverage.MeasureTests(*perTest, filepath.Join(run, "tests"), sources, log))
	}
	keepLatest(run, out)
	return report, nil
}

// unmeasured is a problem for each build root or report coverage did not
// measure: a missing tool or report is the environment, tests that measured
// nothing say no, and an unreadable --coverage-report is a usage error.
func unmeasured(report *coverage.Report) []*problem {
	var out []*problem
	for _, m := range relativeUnmeasured(report) {
		var p *problem
		switch m.Cause {
		case coverage.ToolMissing:
			p = fail(kindMissing, "coverage.tool-missing", "no coverage for "+m.String(), "Install what it names, or measure with --coverage-command.")
		case coverage.MeasuredNothing:
			p = fail(kindNo, "coverage.measured-nothing", "no coverage for "+m.String(), "Run the tests and fix them, then run itos-cc again.")
		case coverage.NoReport:
			p = fail(kindMissing, "coverage.no-report", "no coverage for "+m.String(), "Run without --use-existing-coverage, or write the report first.")
		default:
			p = fail(kindUsage, "coverage.report-unreadable", "no coverage for "+m.String(), "Check the --coverage-report path and its format.")
		}
		if m.Dir != "" {
			p.with("dir", m.Dir)
		}
		if m.Language != "" {
			p.with("language", m.Language)
		}
		if m.Report != "" {
			p.with("report", m.Report)
		}
		out = append(out, p)
	}
	return out
}

// relativeUnmeasured is report's Missing with directories relative to the
// working directory, as every other path itos-cc prints is.
func relativeUnmeasured(report *coverage.Report) []coverage.Unmeasured {
	out := []coverage.Unmeasured{}
	for _, m := range report.Missing() {
		if m.Dir != "" {
			m.Dir = project.Rel(m.Dir)
		}
		out = append(out, m)
	}
	return out
}

// removeAbandoned deletes run directories a killed run left behind: any a
// day old, longer than a run takes.
func removeAbandoned(out string) {
	runs, _ := filepath.Glob(filepath.Join(out, "run-*"))
	for _, r := range runs {
		if info, err := os.Stat(r); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.RemoveAll(r)
		}
	}
}

// keepLatest moves each report directory a run wrote into out, replacing the
// one an earlier run left. It is a cache, so failures are ignored.
func keepLatest(run, out string) {
	entries, _ := os.ReadDir(run)
	for _, e := range entries {
		target := filepath.Join(out, e.Name())
		os.RemoveAll(target)
		os.Rename(filepath.Join(run, e.Name()), target)
	}
}

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
