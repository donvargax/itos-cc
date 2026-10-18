package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// What crap's snapshot records of every function, and the threshold it holds
// them to.

// recordsEveryFunction is whether .metrics/crap.json records exactly the
// functions of the project.
func (c *crapState) recordsEveryFunction() error {
	recorded, err := c.snapshot()
	if err != nil {
		return err
	}
	var got, want []string
	for _, e := range recorded {
		got = append(got, filepath.ToSlash(e.File)+"#"+e.Name)
	}
	for _, f := range c.functions {
		want = append(want, f.file+"#"+f.name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if len(want) == 0 || !slices.Equal(got, want) {
		return fmt.Errorf(".metrics/crap.json records %v, not %v\n%s", got, want, c.w.report())
	}
	return nil
}

// eachHas is whether objects, one at least, each have every key of list
// ("a, b, and c"), with a value that is not empty where nonEmpty.
func eachHas(what string, objects []map[string]any, list string, nonEmpty bool) error {
	if len(objects) == 0 {
		return fmt.Errorf("the object lists no %s", what)
	}
	for i, one := range objects {
		for _, key := range keyList(list) {
			value, ok := one[key]
			if !ok || nonEmpty && (value == nil || value == "") {
				return fmt.Errorf("%s %d has no %q: %v", what, i+1, key, one)
			}
		}
	}
	return nil
}

// eachEntryHas is whether the object lists entries, each with every key
// the list names.
func (c *crapState) eachEntryHas(list string) error {
	var object struct{ Entries []map[string]any }
	if err := json.Unmarshal([]byte(c.w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not crap's object: %v\n%s", err, c.w.report())
	}
	if err := eachHas("entry", object.Entries, list, false); err != nil {
		return fmt.Errorf("%v\n%s", err, c.w.report())
	}
	return nil
}

// eachUnmeasuredHas is whether the object lists unmeasured build roots,
// each giving every key the list names.
func (c *crapState) eachUnmeasuredHas(list string) error {
	var object crapObject
	if err := json.Unmarshal([]byte(c.w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not crap's object: %v\n%s", err, c.w.report())
	}
	if err := eachHas("unmeasured build root", object.Unmeasured, list, true); err != nil {
		return fmt.Errorf("%v\n%s", err, c.w.report())
	}
	return nil
}

// holdsEveryScored is whether .metrics/crap.json records each function the
// table scores, one at least, with the score it shows.
func (c *crapState) holdsEveryScored() error {
	w := c.w
	rows, err := c.table()
	if err != nil {
		return err
	}
	recorded, err := c.snapshot()
	if err != nil {
		return err
	}
	n := 0
	for _, row := range rows {
		if row.CRAP == nil {
			continue
		}
		n++
		if !slices.ContainsFunc(recorded, func(e crapEntry) bool {
			return e.id() == row.id() && filepath.ToSlash(e.File) == filepath.ToSlash(row.File) &&
				e.StartLine == row.StartLine && shown(e.CRAP) == shown(row.CRAP)
		}) {
			return fmt.Errorf(".metrics/crap.json does not record %s scoring %s\n%s", row.shown, shown(row.CRAP), w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf("no function is scored\n%s", w.report())
	}
	return nil
}

// uncoveredComplexity is the complexity at which a function no test runs
// scores score, cc² + cc.
func uncoveredComplexity(score float64) (int, error) {
	cc := int(math.Round((math.Sqrt(1+4*score) - 1) / 2))
	if float64(cc*cc+cc) != score {
		return 0, fmt.Errorf("no function of whole complexity scores %v untested", score)
	}
	return cc, nil
}

// functionScores writes a Go module whose function place, never run,
// scores score; score, never run, one decision more, scores above it; and
// half, run, scores 1. It checks crap scores place so.
func (c *crapState) functionScores(score string) error {
	value, err := strconv.ParseFloat(score, 64)
	if err != nil {
		return err
	}
	cc, err := uncoveredComplexity(value)
	if err != nil {
		return err
	}
	if err := c.goProject(".", []goFunction{
		{name: "place", cc: cc},
		{name: "score", cc: cc + 1},
		{name: "half", cc: 1, covered: true},
	}); err != nil {
		return err
	}
	entries, err := c.measured()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "place" && shown(e.CRAP) == score {
			return nil
		}
	}
	return fmt.Errorf("place does not score %s: %+v", score, entries)
}

// measured is what crap --json scores of the project with its tools.
func (c *crapState) measured() ([]crapEntry, error) {
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--json")
	if err != nil {
		return nil, err
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 {
		return nil, fmt.Errorf("the project could not be scored: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	return object.Entries, nil
}

// threshold is the --threshold of the last run.
func (c *crapState) threshold() (float64, error) {
	if c.w.last != nil {
		for i, a := range c.w.last.args {
			if a == "--threshold" && i+1 < len(c.w.last.args) {
				return strconv.ParseFloat(c.w.last.args[i+1], 64)
			}
		}
	}
	return 0, errors.New("the last run had no --threshold")
}

// saysSoForEvery is whether the last run reports a problem of rule, with
// every key the list names, for each function scoring above its
// threshold, two at least, and none other, its stderr printing each
// problem's message, which names the function and its score.
func (c *crapState) saysSoForEvery(rule, list string) error {
	threshold, err := c.threshold()
	if err != nil {
		return err
	}
	object, report, err := c.lastJSON()
	if err != nil {
		return err
	}
	var problems []map[string]any
	for _, p := range object.Problems {
		if p["rule"] == rule {
			problems = append(problems, p)
		}
	}
	over := 0
	for _, e := range object.Entries {
		if e.CRAP == nil || *e.CRAP <= threshold {
			continue
		}
		over++
		i := slices.IndexFunc(problems, func(p map[string]any) bool { return p["function"] == e.id() })
		if i < 0 {
			return fmt.Errorf("no %s problem names %s, scoring %s\n%s", rule, e.id(), shown(e.CRAP), report)
		}
		if err := eachHas(rule+" problem", problems[i:i+1], list, true); err != nil {
			return fmt.Errorf("%v\n%s", err, report)
		}
		message, _ := problems[i]["message"].(string)
		if !strings.Contains(message, e.id()+" scores "+shown(e.CRAP)) || !strings.Contains(c.w.stderr, "itos-cc: "+message) {
			return fmt.Errorf("stderr does not say %s scores %s in the problem's message %q\n%s", e.id(), shown(e.CRAP), message, report)
		}
	}
	switch {
	case over < 2:
		return fmt.Errorf("%d functions score above %v, not every one of two or more\n%s", over, threshold, report)
	case len(problems) != over:
		return fmt.Errorf("%d %s problems for %d functions above %v\n%s", len(problems), rule, over, threshold, report)
	}
	return nil
}

// everyFunctionAtMost writes a Go module whose functions score limit at
// most, one of them exactly, and checks crap scores them so.
func (c *crapState) everyFunctionAtMost(limit int) error {
	cc, err := uncoveredComplexity(float64(limit))
	if err != nil {
		return err
	}
	if err := c.goProject(".", []goFunction{
		{name: "place", cc: cc},
		{name: "score", cc: 2},
		{name: "half", cc: 3, covered: true},
	}); err != nil {
		return err
	}
	entries, err := c.measured()
	if err != nil {
		return err
	}
	top := 0.0
	for _, e := range entries {
		if e.CRAP == nil {
			return fmt.Errorf("%s is not scored: %+v", e.id(), entries)
		}
		top = max(top, *e.CRAP)
	}
	if top != float64(limit) {
		return fmt.Errorf("the highest score is %v, not %d: %+v", top, limit, entries)
	}
	return nil
}
