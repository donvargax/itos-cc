package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The crap report's table and its rows, and the readers of its output and
// snapshot.

// keyList is a list as a sentence writes it, "a, b, and c", one word each.
func keyList(list string) []string {
	return strings.Split(strings.NewReplacer(", and ", ", ", " and ", ", ").Replace(list), ", ")
}

// tableWithColumns is whether stdout is crap's table: a header of the
// columns the list names, then a row for each function.
func (c *crapState) tableWithColumns(list string) error {
	w := c.w
	lines := outputLines(strings.ReplaceAll(w.stdout, "\r\n", "\n"))
	if got := strings.Fields(lines[0]); !slices.Equal(got, keyList(list)) {
		return fmt.Errorf("the table's columns are %v, not %v\n%s", got, keyList(list), w.report())
	}
	rows, err := c.table()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("the table has no row\n%s", w.report())
	}
	return nil
}

// rowsName is whether every row names its function as format does, its
// words namespace, name, file and line standing for the function's, which
// .metrics/crap.json records there.
func (c *crapState) rowsName(format string) error {
	w := c.w
	pattern := regexp.QuoteMeta(format)
	for _, word := range []struct{ word, group string }{
		{"namespace", `\x00`}, {"name", `(\S+)`}, {"file", `(\S+)`}, {"line", `(\d+)`}, {`\x00`, `(\S*)`},
	} {
		pattern = strings.Replace(pattern, word.word, word.group, 1)
	}
	named := regexp.MustCompile("^" + pattern + "$")
	rows, err := c.table()
	if err != nil {
		return err
	}
	recorded, err := c.snapshot()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("the table has no row\n%s", w.report())
	}
	for _, row := range rows {
		m := named.FindStringSubmatch(row.shown)
		if m == nil {
			return fmt.Errorf("the row naming %q does not name it as %q\n%s", row.shown, format, w.report())
		}
		line, _ := strconv.Atoi(m[4])
		if !slices.ContainsFunc(recorded, func(e crapEntry) bool {
			return e.Namespace == m[1] && e.Name == m[2] && filepath.ToSlash(e.File) == filepath.ToSlash(m[3]) && e.StartLine == line
		}) {
			return fmt.Errorf("%q names no function .metrics/crap.json records\n%s", row.shown, w.report())
		}
	}
	return nil
}

// rowsSorted is whether the rows' scores, of which there are two or more
// different, only fall from the first, the rows with none after them.
func (c *crapState) rowsSorted() error {
	w := c.w
	rows, err := c.table()
	if err != nil {
		return err
	}
	scores := map[float64]bool{}
	for i, row := range rows {
		if row.CRAP != nil {
			scores[*row.CRAP] = true
		}
		if i == 0 {
			continue
		}
		before := rows[i-1]
		if before.CRAP == nil && row.CRAP != nil || before.CRAP != nil && row.CRAP != nil && *row.CRAP > *before.CRAP {
			return fmt.Errorf("%s, scoring %s, comes after %s, scoring %s\n%s", row.id(), shown(row.CRAP), before.id(), shown(before.CRAP), w.report())
		}
	}
	if len(scores) < 2 {
		return fmt.Errorf("the rows have fewer than two different scores to sort\n%s", w.report())
	}
	return nil
}

// unscoredFunction writes a Go module its test measures and the Kotlin
// module coverage cannot measure, whose functions no report applies to.
func (c *crapState) unscoredFunction() error {
	if err := c.goProject(".", []goFunction{{name: "place", cc: 3}, {name: "half", cc: 1, covered: true}}); err != nil {
		return err
	}
	return c.unmeasuredModule()
}

// unscoredRows are the rows of the functions no report applies to, one at
// least.
func (c *crapState) unscoredRows() ([]crapEntry, error) {
	if c.unscored == "" {
		return nil, errors.New("no function was written that no coverage report applies to")
	}
	return c.rowsOf(c.unscored)
}

// rowsOf is the rows of the functions of file, slash-separated from the
// scratch folder, from the last run's table or object; one at least.
func (c *crapState) rowsOf(file string) ([]crapEntry, error) {
	entries, err := c.listed()
	if err != nil {
		return nil, err
	}
	var out []crapEntry
	for _, e := range entries {
		if filepath.ToSlash(e.File) == file {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no function of %s is listed\n%s", file, c.w.report())
	}
	return out, nil
}

// unscoredShownAs is whether the rows of the functions no report applies
// to show text as their CRAP and COV%.
func (c *crapState) unscoredShownAs(text string) error {
	rows, err := c.unscoredRows()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if shown(row.CRAP) != text || shown(row.Coverage) != text {
			return fmt.Errorf("%s shows %s and %s, not %s\n%s", row.id(), shown(row.CRAP), shown(row.Coverage), text, c.w.report())
		}
	}
	return nil
}

// unscoredLast is whether the rows of the functions no report applies to
// come after every scored row, and the rows with no score fall in
// complexity, of which there are two or more different among them.
func (c *crapState) unscoredLast() error {
	w := c.w
	if _, err := c.unscoredRows(); err != nil {
		return err
	}
	rows, err := c.table()
	if err != nil {
		return err
	}
	lastScored, firstUnscored := -1, len(rows)
	complexities := map[int]bool{}
	for i, row := range rows {
		if row.CRAP != nil {
			lastScored = i
			continue
		}
		complexities[row.Complexity] = true
		if filepath.ToSlash(row.File) == c.unscored {
			firstUnscored = min(firstUnscored, i)
		}
		if i > 0 && rows[i-1].CRAP == nil && row.Complexity > rows[i-1].Complexity {
			return fmt.Errorf("%s, of complexity %d, comes after %s, of %d\n%s", row.id(), row.Complexity, rows[i-1].id(), rows[i-1].Complexity, w.report())
		}
	}
	switch {
	case lastScored < 0:
		return fmt.Errorf("no function is scored to come after\n%s", w.report())
	case len(complexities) < 2:
		return fmt.Errorf("the functions with no score have fewer than two different complexities to order\n%s", w.report())
	case firstUnscored < lastScored:
		return fmt.Errorf("%s comes before a scored function\n%s", rows[firstUnscored].id(), w.report())
	}
	return nil
}

// table is the rows of the table the last run printed, past its header.
func (c *crapState) table() ([]crapEntry, error) {
	w := c.w
	if strings.HasPrefix(w.stdout, "{") || w.stdout == "" {
		return nil, fmt.Errorf("standard output is no table\n%s", w.report())
	}
	var rows []crapEntry
	for _, line := range outputLines(strings.ReplaceAll(w.stdout, "\r\n", "\n"))[1:] {
		m := crapRow.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("%q is no row of the table\n%s", line, w.report())
		}
		row := crapEntry{shown: m[4]}
		row.Complexity, _ = strconv.Atoi(m[2])
		for _, column := range []struct {
			text string
			into **float64
		}{{m[1], &row.CRAP}, {m[3], &row.Coverage}} {
			if column.text == "N/A" {
				continue
			}
			value, err := strconv.ParseFloat(column.text, 64)
			if err != nil {
				return nil, fmt.Errorf("%q is no score in the row %q\n%s", column.text, line, w.report())
			}
			*column.into = &value
		}
		if f := rowFunction.FindStringSubmatch(m[4]); f != nil {
			row.Namespace, row.Name, row.File = f[1], f[2], filepath.ToSlash(f[3])
			row.StartLine, _ = strconv.Atoi(f[4])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// listed is the functions the last run listed, from its --json object or
// its table.
func (c *crapState) listed() ([]crapEntry, error) {
	w := c.w
	if !strings.HasPrefix(w.stdout, "{") {
		return c.table()
	}
	var object crapObject
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not crap's object: %v\n%s", err, w.report())
	}
	return object.Entries, nil
}

// snapshot is the functions .metrics/crap.json records, written by the last
// run in the scratch folder.
func (c *crapState) snapshot() ([]crapEntry, error) {
	data, err := c.w.readIn(c.w.dir, ".metrics/crap.json")
	if err != nil {
		return nil, fmt.Errorf("no .metrics/crap.json: %v\n%s", err, c.w.report())
	}
	var recorded struct{ Entries []crapEntry }
	if err := json.Unmarshal(data, &recorded); err != nil {
		return nil, fmt.Errorf(".metrics/crap.json is not crap's snapshot: %v", err)
	}
	return recorded.Entries, nil
}

// lastJSON is the object of the last run, made again with --json before
// its words, as a script would make it, unless it had --json already: that
// run must exit as the plain one did. The report is both runs'.
func (c *crapState) lastJSON() (crapObject, string, error) {
	w := c.w
	if w.last == nil {
		return crapObject{}, "", errors.New("no run of itos-cc to read")
	}
	stdout, report := w.stdout, w.report()
	if !slices.Contains(w.last.args, "--json") {
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, append([]string{"--json"}, w.last.args...)...)
		if err != nil {
			return crapObject{}, "", err
		}
		report = fmt.Sprintf("%s\n--- with --json, exit %d\n--- stdout\n%s--- stderr\n%s", report, code, out, errOut)
		if code != w.exit {
			return crapObject{}, "", fmt.Errorf("with --json itos-cc exited %d, not %d as without it\n%s", code, w.exit, report)
		}
		stdout = out
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return crapObject{}, "", fmt.Errorf("the output with --json is not crap's object: %v\n%s", err, report)
	}
	return object, report, nil
}
