package features

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The scrap report as printed and recorded, read back, and the clusters it
// holds.

// The text output, line by line: a file's line, its warning, its
// recommendations and, with --verbose, its examples.
var (
	fileLine = regexp.MustCompile(`^(\S+)  (LEAVE_ALONE|AUTO_TABLE_DRIVE|AUTO_REFACTOR|MANUAL_SPLIT|REVIEW_FIRST)  ` +
		`pressure=(\d+\.\d)  examples=(\d+)  avg=(\d+\.\d)  max=(\d+\.\d)(?:  (improved|worse) from (\d+\.\d))?$`)
	recommendationLine = regexp.MustCompile(`^  (HIGH  |MEDIUM|LOW   ) (.+)$`)
	exampleLine        = regexp.MustCompile(`^    ( *\d+\.\d)  (\S+):(\d+)-(\d+) (".*") lines=(\d+) assertions=(\d+) decisions=(\d+) mocks=(\d+) \[(.*)\]$`)
)

// printedFile is what the text output prints of one file.
type printedFile struct {
	fields          []string // fileLine's submatches
	line            string
	warnings        []string
	recommendations []scrapRecommendation
	examples        [][]string // exampleLine's submatches
}

// printed reads the text output, every line a file's or one under it.
func printed(stdout string) ([]printedFile, error) {
	var files []printedFile
	for _, line := range outputLines(filepath.ToSlash(stdout)) {
		line = strings.TrimSuffix(line, "\r")
		if m := fileLine.FindStringSubmatch(line); m != nil {
			files = append(files, printedFile{fields: m, line: line})
			continue
		}
		if line == "" && len(files) == 0 {
			continue
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%q comes before any file's line", line)
		}
		f := &files[len(files)-1]
		switch m := recommendationLine.FindStringSubmatch(line); {
		case m != nil:
			f.recommendations = append(f.recommendations, scrapRecommendation{strings.TrimSpace(m[1]), m[2]})
		case exampleLine.MatchString(line):
			f.examples = append(f.examples, exampleLine.FindStringSubmatch(line))
		case strings.HasPrefix(line, "  the last change"):
			f.warnings = append(f.warnings, line)
		default:
			return nil, fmt.Errorf("%q is no line of a file's report", line)
		}
	}
	return files, nil
}

// reported are the reports of the scenario's run by file: from its --json
// object, or with the text output from the .metrics/scrap.json it wrote,
// the text printing a line of each with its action, pressure, examples and
// scores.
func (d *scrapState) reported() (map[string]scrapReport, error) {
	w := d.w
	if w.last == nil {
		return nil, errors.New("itos-cc scrap was not run")
	}
	if d.reportedRun != w.last {
		d.reportedRun = w.last
		d.reportedFiles, d.reportedErr = d.readReported()
	}
	return d.reportedFiles, d.reportedErr
}

func (d *scrapState) readReported() (map[string]scrapReport, error) {
	w := d.w
	if w.exit != 0 {
		return nil, fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		return reportsOf(w.stdout)
	}
	snapshot, err := d.snapshot()
	if err != nil {
		return nil, err
	}
	files, err := printed(w.stdout)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, w.report())
	}
	reports := map[string]scrapReport{}
	for _, f := range files {
		r, ok := snapshot[f.fields[1]]
		if !ok {
			return nil, fmt.Errorf("%s is printed, and .metrics/scrap.json has no report of it", f.fields[1])
		}
		want := []string{r.Action, fmt.Sprintf("%.1f", r.Pressure), strconv.Itoa(r.Examples), fmt.Sprintf("%.1f", r.AverageScore), fmt.Sprintf("%.1f", r.MaxScore)}
		if !slices.Equal(f.fields[2:7], want) {
			return nil, fmt.Errorf("%q is printed, and .metrics/scrap.json holds %v of it", f.line, want)
		}
		reports[r.File] = r
	}
	return reports, nil
}

// snapshot is what .metrics/scrap.json holds, by file.
func (d *scrapState) snapshot() (map[string]scrapReport, error) {
	raw, err := d.w.readIn(d.w.dir, ".metrics/scrap.json")
	if err != nil {
		return nil, fmt.Errorf("no .metrics/scrap.json: %v", err)
	}
	return reportsOf(string(raw))
}

// its is the report of the scenario's file, or of the one file the run
// reported.
func (d *scrapState) its() (scrapReport, error) {
	reports, err := d.reported()
	if err != nil {
		return scrapReport{}, err
	}
	if d.file != "" {
		r, ok := reports[d.file]
		if !ok {
			return r, fmt.Errorf("the run reports nothing of %s\n%s", d.file, d.w.report())
		}
		return r, nil
	}
	if len(reports) != 1 {
		return scrapReport{}, fmt.Errorf("the run reports %d files, not one\n%s", len(reports), d.w.report())
	}
	for _, r := range reports {
		return r, nil
	}
	return scrapReport{}, nil
}

// hasClusters is whether the file has count clusters of the kind, each of
// that many examples.
func (d *scrapState) hasClusters(count, kind string, examples int) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	n, err := countOf(count)
	if err != nil {
		return err
	}
	var sizes []int
	for _, c := range r.Clusters {
		if c.Kind == kind {
			sizes = append(sizes, len(c.Examples))
		}
	}
	if len(sizes) != n || slices.ContainsFunc(sizes, func(s int) bool { return s != examples }) {
		return fmt.Errorf("%s has %q clusters of %v examples, not %d of %d: %+v", r.File, kind, sizes, n, examples, r.Clusters)
	}
	return nil
}

// firstFolds is whether the first recommendation folds the examples of the
// file's table cluster, naming each, into the idiom.
func (d *scrapState) firstFolds(idiom string) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	if len(r.Recommendations) == 0 {
		return fmt.Errorf("%s has no recommendation", r.File)
	}
	first := r.Recommendations[0].Text
	if !strings.Contains(first, "fold them into "+idiom) {
		return fmt.Errorf("the first recommendation %q does not say to fold them into %s", first, idiom)
	}
	for _, c := range r.Clusters {
		if c.Kind != "table" {
			continue
		}
		for _, e := range c.Examples {
			if !strings.Contains(first, strconv.Quote(e.Name)) {
				return fmt.Errorf("the first recommendation %q does not name %q", first, e.Name)
			}
		}
		return nil
	}
	return fmt.Errorf("%s has no table cluster", r.File)
}
