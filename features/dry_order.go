package features

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The order and groups dry reports its pairs in, and what its JSON and
// snapshot hold.

// ordered are groups of TypeScript functions in a folder of their own: in
// c1 to c3 three copies of totalTs, in b1 and b2 two of loadTs, both groups
// scoring 1, and in a1 and a2 fetchTs and a copy calling head for get,
// scoring 0.903. Best first, then largest first, is the reverse of their
// files' order.
var ordered = map[string]string{
	"a1.ts": fetchTs, "a2.ts": strings.Replace(fetchTs, "await get(", "await head(", 1),
	"b1.ts": loadTs, "b2.ts": loadTs,
	"c1.ts": totalTs, "c2.ts": totalTs, "c3.ts": totalTs,
}

// pairsBestFirst is whether the entries and the pairs are listed from the
// highest score down, the scenario's command run over ordered.
func (d *dryState) pairsBestFirst() error {
	listed, object, err := d.otherFolder("ordered", ordered)
	if err != nil {
		return err
	}
	var entryScores, pairScores []float64
	for _, e := range listed {
		entryScores = append(entryScores, e.max)
	}
	for _, p := range object.Candidates {
		pairScores = append(pairScores, p.Score)
	}
	for _, s := range [][]float64{entryScores, pairScores} {
		if len(slices.Compact(slices.Clone(s))) < 2 {
			return fmt.Errorf("the fixture gives scores %v, too few to order", s)
		}
		if !slices.IsSortedFunc(s, func(a, b float64) int { return cmp.Compare(b, a) }) {
			return fmt.Errorf("listed with scores %v, not best first", s)
		}
	}
	return nil
}

// groupsBestThenLargest is whether the entries are listed by their best
// score, highest first, then by their members, most first, the scenario's
// command run over ordered.
func (d *dryState) groupsBestThenLargest() error {
	listed, _, err := d.otherFolder("ordered", ordered)
	if err != nil {
		return err
	}
	tie := false
	for i := 1; i < len(listed); i++ {
		a, b := listed[i-1], listed[i]
		if a.max == b.max && len(a.members) != len(b.members) {
			tie = true
		}
		if a.max < b.max || a.max == b.max && len(a.members) < len(b.members) {
			return fmt.Errorf("entry %d, %v with %d members, is listed before %v with %d", i, a.max, len(a.members), b.max, len(b.members))
		}
	}
	if !tie || len(listed) < 3 {
		return fmt.Errorf("the fixture lists %d entries, without two of one score and unlike sizes", len(listed))
	}
	return nil
}

// linked are TypeScript functions in a folder of their own: b is totalTs,
// a calls fix for round and c subtracts the tax, each 0.897 with b and
// 0.804 with each other; e, with two changes of its own, is 0.818 with b and
// 0.731 with a and c, so it joins them at any threshold below the default;
// d is unlike them all.
var linked = map[string]string{
	"a.ts": strings.Replace(totalTs, "round(sum", "fix(sum", 1),
	"b.ts": totalTs,
	"c.ts": strings.Replace(totalTs, "sum + tax", "sum - tax", 1),
	"d.ts": loadTs,
	"e.ts": strings.NewReplacer("!line.void", "line.ok", "> 0", ">= 0").Replace(totalTs),
}

// groupHoldsLinked is whether each entry, the scenario's command run over
// linked, holds the functions linked by pairs at or above its threshold,
// every one of them, and no other: what dry scores with nothing left out,
// joined. The fixture must have a group with two members whose own pair is
// below the threshold, and a function in none.
func (d *dryState) groupHoldsLinked() error {
	listed, object, err := d.otherFolder("linked", linked)
	if err != nil {
		return err
	}
	all, err := d.everyPair(filepath.Join(d.w.dir, "linked"))
	if err != nil {
		return err
	}
	group := map[string]string{} // each function's group, by a member's name
	var find func(string) string
	find = func(s string) string {
		if group[s] == "" || group[s] == s {
			return s
		}
		return find(group[s])
	}
	functions := map[string]bool{}
	for _, p := range all {
		functions[p.Left.String()], functions[p.Right.String()] = true, true
		if p.Score >= object.Threshold {
			group[find(p.Left.String())] = find(p.Right.String())
		}
	}
	members := map[string][]string{}
	for f := range functions {
		members[find(f)] = append(members[find(f)], f)
	}
	var want, got []string
	alone := false
	for _, m := range members {
		if len(m) == 1 {
			alone = true
			continue
		}
		slices.Sort(m)
		want = append(want, strings.Join(m, ", "))
	}
	for _, e := range listed {
		m := slices.Clone(e.members)
		slices.Sort(m)
		got = append(got, strings.Join(m, ", "))
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("the groups are\n  %s\nnot\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if !alone {
		return errors.New("the fixture has no function in no group")
	}
	return needs(all, "below the threshold of two members of one group", func(p dryPair) bool {
		return p.Score < object.Threshold && find(p.Left.String()) == find(p.Right.String())
	})
}

// numbers are the counts a scenario writes as words.
var numbers = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6}

// oneEntryNotPairs is whether stdout is the text, one entry, and the run
// reported as many pairs as the scenario says.
func (d *dryState) oneEntryNotPairs(count string, text *godog.DocString) error {
	if err := d.w.stdoutShows(text); err != nil {
		return err
	}
	listed, err := entries(d.w.stdout)
	if err != nil {
		return err
	}
	if len(listed) != 1 {
		return fmt.Errorf("stdout shows %d entries, not one", len(listed))
	}
	got, err := d.reported()
	if err != nil {
		return err
	}
	if want, ok := numbers[count]; !ok || len(got) != want {
		return fmt.Errorf("the run reported %d pairs, not %s:\n  %s", len(got), count, strings.Join(keys(got), "\n  "))
	}
	return nil
}

// snapshotHolds is whether .metrics/dry.json holds a version, and the
// threshold, the candidates and the groups stdout's object holds.
func (d *dryState) snapshotHolds() error {
	w := d.w
	raw, err := w.readIn(w.last.dir, ".metrics/dry.json")
	if err != nil {
		return fmt.Errorf("the run wrote no .metrics/dry.json: %v", err)
	}
	var snapshot, object map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return fmt.Errorf(".metrics/dry.json is no JSON object: %v\n%s", err, raw)
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is no JSON object: %v\n%s", err, w.report())
	}
	if version, ok := snapshot["version"].(float64); !ok || version < 1 {
		return fmt.Errorf(".metrics/dry.json has no version: %v", snapshot["version"])
	}
	// stdout names a file from the working directory with the system's
	// separator, the snapshot with slashes: an escaped backslash in the
	// JSON text reads as a slash.
	for _, key := range []string{"threshold", "candidates", "groups"} {
		want, _ := json.Marshal(object[key])
		got, _ := json.Marshal(snapshot[key])
		if object[key] == nil || string(got) != strings.ReplaceAll(string(want), `\\`, "/") {
			return fmt.Errorf(".metrics/dry.json's %s is %s, not stdout's %s", key, got, want)
		}
	}
	return nil
}

// sentenceKeys are the keys a sentence lists ("a, b, and c"), each of "lines"
// the start and end lines.
func sentenceKeys(list string) []string {
	var out []string
	for _, key := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(list, ", and ", ", "), " and ", ", "), ", ") {
		if key == "lines" {
			out = append(out, "start_line", "end_line")
			continue
		}
		out = append(out, key)
	}
	return out
}

// jsonList is the list key of stdout's object, which must hold something.
func (d *dryState) jsonList(key string) ([]map[string]any, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(d.w.stdout), &raw); err != nil {
		return nil, fmt.Errorf("standard output is no JSON object: %v\n%s", err, d.w.report())
	}
	var list []map[string]any
	if err := json.Unmarshal(raw[key], &list); err != nil || len(list) == 0 {
		return nil, fmt.Errorf("the object's %s is no list of objects with something in it: %v\n%s", key, err, d.w.report())
	}
	return list, nil
}

// hasKeys is whether the object has every key, what a sentence calls it
// for the error.
func hasKeys(object map[string]any, what string, keys []string) error {
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s has no %q: %v", what, key, object)
		}
	}
	return nil
}

// eachCandidateHas is whether stdout's object lists candidates, each with
// every key of first and a left and a right side with every key of sides.
func (d *dryState) eachCandidateHas(first, sides string) error {
	candidates, err := d.jsonList("candidates")
	if err != nil {
		return err
	}
	for i, c := range candidates {
		if err := hasKeys(c, fmt.Sprintf("candidate %d", i+1), sentenceKeys(first)); err != nil {
			return err
		}
		for _, side := range []string{"left", "right"} {
			s, ok := c[side].(map[string]any)
			if !ok {
				return fmt.Errorf("candidate %d has no %s side: %v", i+1, side, c)
			}
			if err := hasKeys(s, fmt.Sprintf("candidate %d's %s side", i+1, side), sentenceKeys(sides)); err != nil {
				return err
			}
		}
	}
	return nil
}

// eachGroupHas is whether stdout's object lists groups, each with every
// key of list and members, a list with something in it.
func (d *dryState) eachGroupHas(list string) error {
	groups, err := d.jsonList("groups")
	if err != nil {
		return err
	}
	for i, g := range groups {
		if err := hasKeys(g, fmt.Sprintf("group %d", i+1), sentenceKeys(list)); err != nil {
			return err
		}
		if members, ok := g["members"].([]any); !ok || len(members) == 0 {
			return fmt.Errorf("group %d has no members: %v", i+1, g)
		}
	}
	return nil
}
