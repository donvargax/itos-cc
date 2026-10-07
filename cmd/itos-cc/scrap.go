package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
	"github.com/donvargax/itos-cc/scrap"
)

type scrapSnapshot struct {
	Version int                `json:"version"`
	Files   []scrap.FileReport `json:"files"`
}

var scrapCommand = &command{
	name:     "scrap",
	summary:  "test-code structure: which tests to leave, table-drive, refactor, or split",
	synopsis: "[options] [path ...]",
	about: `
Measures test code: examples that are too large, branch or loop, mock heavily,
assert nothing or too much, or repeat each other. Each file gets an action
for an assistant:

  LEAVE_ALONE       fine as it is; change it only if asked
  AUTO_TABLE_DRIVE  fold repeated examples into one table-driven test
  AUTO_REFACTOR     clean up the listed examples in place
  MANUAL_SPLIT      split the file by responsibility first, then rerun
  REVIEW_FIRST      unstable, but not clear enough to change unattended

Recommendations are decision support, not orders: check each against what
the test is for before changing it. Each run is compared with the previous
.metrics/scrap.json, so rerun after a refactor to see whether it helped.`,
	flags: append(append([]flagSpec{}, selectionFlags...),
		sw("verbose", "print every example's measurements")),
	json: `"files": [{"file", "language", "examples", "action", "pressure",
   "average_score", "max_score", "recommendations", "clusters", "details",
   "compare"}]`,
	exits: []exitDoc{
		{0, "success, whatever the actions"},
		{2, "a usage error: a bad flag or path"},
		{3, "--changed outside a git repository"},
	},
	examples: []string{
		"itos-cc scrap",
		"itos-cc scrap --changed --verbose",
	},
	run: runScrap,
}

type scrapResult struct {
	Files []scrap.FileReport `json:"files"`
}

func runScrap(in *invocation) (any, error) {
	result := scrapResult{Files: []scrap.FileReport{}}
	files, err := files(in)
	if err != nil {
		return result, err
	}
	if len(files.Tests) == 0 {
		fmt.Fprintln(os.Stderr, "itos-cc: no test files to measure")
		return result, nil
	}

	var previous scrapSnapshot
	if _, err := metrics.Read("scrap.json", &previous); err != nil {
		fmt.Fprintln(os.Stderr, "itos-cc: ignoring unreadable .metrics/scrap.json:", err)
	}
	byFile := map[string]scrap.FileReport{}
	for _, r := range previous.Files {
		byFile[r.File] = r
	}

	reports := []scrap.FileReport{}
	for _, path := range files.Tests {
		f, err := lang.ParseFile(path)
		if err != nil {
			return result, err
		}
		r := scrap.Analyze(f, project.Rel(path))
		f.Close()
		if r.Examples == 0 {
			continue
		}
		if prev, ok := byFile[r.File]; ok {
			prev.Compare = nil
			r.CompareTo(prev)
		}
		byFile[r.File] = r
		reports = append(reports, r)
	}

	snapshot := scrapSnapshot{Version: metrics.Version, Files: []scrap.FileReport{}}
	for _, r := range byFile {
		r.Compare = nil
		snapshot.Files = append(snapshot.Files, r)
	}
	sort.Slice(snapshot.Files, func(i, j int) bool { return snapshot.Files[i].File < snapshot.Files[j].File })
	if err := metrics.Write("scrap.json", snapshot); err != nil {
		return result, err
	}

	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Pressure > reports[j].Pressure })
	result.Files = reports
	if !in.json {
		for _, r := range reports {
			printScrap(r, in.set("verbose"))
		}
	}
	return result, nil
}

func printScrap(r scrap.FileReport, verbose bool) {
	change := ""
	if c := r.Compare; c != nil && c.Verdict != "unchanged" {
		change = fmt.Sprintf("  %s from %.1f", c.Verdict, c.PreviousPressure)
	}
	fmt.Printf("%s  %s  pressure=%.1f  examples=%d  avg=%.1f  max=%.1f%s\n",
		r.File, r.Action, r.Pressure, r.Examples, r.AverageScore, r.MaxScore, change)
	if c := r.Compare; c != nil && c.Verdict == "worse" {
		fmt.Println("  the last change made this file worse: check new helpers and duplication before keeping it")
	}
	// A file to leave alone keeps its minor hints out of the way.
	if r.Action != scrap.LeaveAlone || verbose {
		for _, rec := range r.Recommendations {
			fmt.Printf("  %-6s %s\n", rec.Confidence, rec.Text)
		}
	}
	if verbose {
		for _, e := range r.Details {
			fmt.Printf("    %5.1f  %s:%d-%d %q lines=%d assertions=%d decisions=%d mocks=%d %v\n",
				e.Score, r.File, e.StartLine, e.EndLine, e.Name, e.Lines, e.Assertions, e.Decisions, e.Mocks, e.Smells)
		}
	}
}
