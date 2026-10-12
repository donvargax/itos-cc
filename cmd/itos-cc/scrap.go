package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
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

	ctx := context.Background()
	var previous scrapSnapshot
	// An unreadable last snapshot is only warned about, and the run goes on.
	if _, failure := snapshots.Read(ctx, "scrap.json", &previous); failure != nil {
		var unreadable *snapport.Unreadable
		if !errors.As(failure, &unreadable) {
			return result, failure
		}
		fmt.Fprintln(os.Stderr, "itos-cc: ignoring unreadable .metrics/scrap.json:", failure)
	}
	root, failure := snapshots.Root(ctx)
	if failure != nil {
		return result, failure
	}
	// scrap.json names each file from the project root, while output names
	// it from the working directory.
	byFile := map[string]scrap.FileReport{}
	for _, r := range previous.Files {
		byFile[r.File] = r
	}

	reports := []scrap.FileReport{}
	for _, path := range files.Tests {
		f, err := project.ParseFile(context.TODO(), path)
		if err != nil {
			return result, err
		}
		r := scrap.Analyze(f, project.Rel(path))
		f.Close()
		if r.Examples == 0 {
			continue
		}
		key := snapshot.Key(root, path)
		if prev, ok := byFile[key]; ok {
			prev.Compare = nil
			r.CompareTo(prev)
		}
		recorded := r
		recorded.File = key
		byFile[key] = recorded
		reports = append(reports, r)
	}

	recorded := scrapSnapshot{Version: snapshot.Version, Files: []scrap.FileReport{}}
	for _, r := range byFile {
		r.Compare = nil
		recorded.Files = append(recorded.Files, r)
	}
	sort.Slice(recorded.Files, func(i, j int) bool { return recorded.Files[i].File < recorded.Files[j].File })
	if failure := snapshots.Write(ctx, "scrap.json", recorded); failure != nil {
		return result, failure
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
