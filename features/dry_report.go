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

// The pairs, groups and entries dry reports, read back from its output.

// drySide is one function of a pair or member of a group, as --json and
// .metrics/dry.json give it.
type drySide struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
}

func (s drySide) lines() int { return s.EndLine - s.StartLine + 1 }

// String is the side as the text output lists it.
func (s drySide) String() string {
	return fmt.Sprintf("%s:%d-%d  %s#%s", filepath.ToSlash(s.File), s.StartLine, s.EndLine, s.Namespace, s.Name)
}

type dryPair struct {
	Score    float64 `json:"score"`
	Language string  `json:"language"`
	Left     drySide `json:"left"`
	Right    drySide `json:"right"`
}

func (p dryPair) String() string { return fmt.Sprintf("%v %s | %s", p.Score, p.Left, p.Right) }

// key is the pair whatever its order.
func (p dryPair) key() string {
	a, b := p.Left.String(), p.Right.String()
	if b < a {
		a, b = b, a
	}
	return a + " | " + b
}

// in is whether a side of the pair is in one of files.
func (p dryPair) in(files []string) bool {
	return slices.Contains(files, filepath.ToSlash(p.Left.File)) || slices.Contains(files, filepath.ToSlash(p.Right.File))
}

// both is whether both sides of the pair are what keep says.
func (p dryPair) both(keep func(drySide) bool) bool { return keep(p.Left) && keep(p.Right) }

type dryGroup struct {
	Language string    `json:"language"`
	MinScore float64   `json:"min_score"`
	MaxScore float64   `json:"max_score"`
	Members  []drySide `json:"members"`
}

// dryObject is dry's --json object.
type dryObject struct {
	Threshold  float64    `json:"threshold"`
	Candidates []dryPair  `json:"candidates"`
	Groups     []dryGroup `json:"groups"`
}

// entry is one entry of the text output: its score, a range where the
// group's pairs differ, its language and its members' lines.
type entry struct {
	min, max float64
	language string
	members  []string
}

var (
	entryHead   = regexp.MustCompile(`^DUPLICATE score=(\d\.\d\d)(?:–(\d\.\d\d))? (\S+)$`)
	entryMember = regexp.MustCompile(`^  (\S.*:\d+-\d+  .*#.+)$`)
)

// entries reads the text output, each line an entry's head or one of its
// members.
func entries(stdout string) ([]entry, error) {
	var all []entry
	for _, line := range outputLines(filepath.ToSlash(stdout)) {
		line = strings.TrimSuffix(line, "\r")
		if m := entryHead.FindStringSubmatch(line); m != nil {
			e := entry{language: m[3]}
			e.min, _ = strconv.ParseFloat(m[1], 64)
			e.max = e.min
			if m[2] != "" {
				e.max, _ = strconv.ParseFloat(m[2], 64)
			}
			all = append(all, e)
			continue
		}
		if m := entryMember.FindStringSubmatch(line); m != nil && len(all) > 0 {
			all[len(all)-1].members = append(all[len(all)-1].members, m[1])
			continue
		}
		if line != "" {
			return nil, fmt.Errorf("%q is no line of an entry", line)
		}
	}
	return all, nil
}

// reported are the pairs the scenario's run reported: from its --json
// object, or with the text output from the .metrics/dry.json it wrote, the
// text listing the functions of those pairs, an entry for each group.
func (d *dryState) reported() ([]dryPair, error) {
	w := d.w
	if w.last == nil {
		return nil, errors.New("itos-cc dry was not run")
	}
	if d.reportedRun != w.last {
		d.reportedRun = w.last
		d.reportedPairs, d.reportedErr = d.readReported()
	}
	return d.reportedPairs, d.reportedErr
}

func (d *dryState) readReported() ([]dryPair, error) {
	w := d.w
	if w.exit != 0 {
		return nil, fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	var object dryObject
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
			return nil, fmt.Errorf("standard output is not dry's object: %v\n%s", err, w.report())
		}
		return object.Candidates, nil
	}
	raw, err := w.readIn(w.last.dir, ".metrics/dry.json")
	if err != nil {
		return nil, fmt.Errorf("the run wrote no .metrics/dry.json: %v\n%s", err, w.report())
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf(".metrics/dry.json is not dry's snapshot: %v\n%s", err, raw)
	}
	listed, err := entries(w.stdout)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, w.report())
	}
	var printed, recorded []string
	for _, e := range listed {
		printed = append(printed, e.members...)
	}
	for _, p := range object.Candidates {
		recorded = append(recorded, p.Left.String(), p.Right.String())
	}
	slices.Sort(printed)
	slices.Sort(recorded)
	if !slices.Equal(printed, slices.Compact(recorded)) || len(listed) != len(object.Groups) {
		return nil, fmt.Errorf("stdout lists %d entries of\n  %s\nand .metrics/dry.json %d groups of\n  %s\n%s",
			len(listed), strings.Join(printed, "\n  "), len(object.Groups), strings.Join(recorded, "\n  "), w.report())
	}
	return object.Candidates, nil
}

// dryIn runs itos-cc dry with args and --json in dir, after reading the
// scenario's own run, and gives its object.
func (d *dryState) dryIn(dir string, args ...string) (dryObject, error) {
	_, _ = d.reported()
	var object dryObject
	args = append([]string{"dry", "--json"}, args...)
	code, stdout, stderr, err := d.w.capture(dir, d.w.env(), args...)
	if err != nil {
		return object, err
	}
	if code != 0 {
		return object, fmt.Errorf("itos-cc %s exited %d\n%s%s", strings.Join(args, " "), code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return object, fmt.Errorf("itos-cc %s printed no JSON object: %v\n%s", strings.Join(args, " "), err, stdout)
	}
	return object, nil
}
