package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The units scenarios about mounted routes and the measures each unit owns.

// measures are what crap, dry and mutation make of each unit of a file, by
// name: its complexity, its mutation sites, its normalized syntax nodes and
// its hash.
type measures struct {
	complexity, sites, nodes map[string]int
	hash                     map[string]string
}

// measure runs crap, mutation list, dry and mutation run over file in dir.
// mutation run's tests are "exit 0", which passes in sh and in cmd alike,
// so every mutant survives and the snapshot holds each unit's hash.
func (u *unitsState) measure(dir, file string) (measures, error) {
	m := measures{complexity: map[string]int{}, sites: map[string]int{}, nodes: map[string]int{}, hash: map[string]string{}}
	var crap struct {
		Entries []struct {
			Name       string
			Complexity int
		}
	}
	if err := u.runJSON(dir, &crap, "crap", "--no-coverage", "--json", file); err != nil {
		return m, err
	}
	for _, e := range crap.Entries {
		m.complexity[e.Name] = e.Complexity
	}
	var list struct{ Sites []struct{ Function string } }
	if err := u.runJSON(dir, &list, "mutation", "list", "--json", file); err != nil {
		return m, err
	}
	for _, s := range list.Sites {
		_, name, _ := strings.Cut(s.Function, "#")
		m.sites[name]++
	}
	type member struct {
		File, Name string
		Nodes      int
	}
	var dry struct {
		Candidates []struct{ Left, Right member }
	}
	if err := u.runJSON(dir, &dry, "dry", "--json", "--threshold", "0", "--min-lines", "1", "--min-nodes", "1", file); err != nil {
		return m, err
	}
	for _, c := range dry.Candidates {
		for _, one := range []member{c.Left, c.Right} {
			if filepath.ToSlash(one.File) == file {
				m.nodes[one.Name] = one.Nodes
			}
		}
	}
	if err := u.w.runEnv(dir, u.w.env(), "mutation", "run", "--test-command", "exit 0", "--json", file); err != nil {
		return m, err
	}
	raw, err := u.w.readIn(dir, ".metrics/mutate/"+file+".json")
	if err != nil {
		return m, fmt.Errorf("mutation run wrote no snapshot of %s: %v\n%s", file, err, u.w.report())
	}
	var snapshot struct{ Units []struct{ Name, Hash string } }
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return m, err
	}
	for _, x := range snapshot.Units {
		m.hash[x.Name] = x.Hash
	}
	return m, nil
}

// runJSON runs itos-cc with args in dir and reads its --json object into
// v, the run having succeeded.
func (u *unitsState) runJSON(dir string, v any, args ...string) error {
	if err := u.w.runEnv(dir, u.w.env(), args...); err != nil {
		return err
	}
	if u.w.exit != 0 {
		return fmt.Errorf("itos-cc %s exited %d\n%s", strings.Join(args, " "), u.w.exit, u.w.report())
	}
	if err := json.Unmarshal([]byte(u.w.stdout), v); err != nil {
		return fmt.Errorf("itos-cc %s printed no JSON object: %v\n%s", strings.Join(args, " "), err, u.w.report())
	}
	return nil
}

// mounted is mount, the unit the scenario's routes are mounted in; the
// first route inside it with a line of its own below the one it starts on;
// and the lines of the routes inside it below the ones they start on, their
// bodies, from a listing of the file.
func (u *unitsState) mounted() (mount, route unit, bodies []int, err error) {
	if err := u.list(u.file); err != nil {
		return mount, route, nil, err
	}
	all, err := u.listed()
	if err != nil {
		return mount, route, nil, err
	}
	for _, x := range all {
		if x.Name == "mount" {
			mount = x
		}
	}
	for _, x := range all {
		if mount.Name == "" || x.Name == "mount" || x.StartLine < mount.StartLine || x.EndLine > mount.EndLine {
			continue
		}
		if route.Name == "" && x.EndLine > x.StartLine {
			route = x
		}
		for line := x.StartLine + 1; line <= x.EndLine; line++ {
			bodies = append(bodies, line)
		}
	}
	if route.Name == "" {
		return mount, route, nil, fmt.Errorf("%s has no route mounted in mount with a body of its own\n%s", u.file, u.w.report())
	}
	return mount, route, bodies, nil
}

// mountRan is a run of mount's lines, between its first and its last, but
// its routes' bodies: each line run once, or none.
func mountRan(mount unit, bodies []int) map[int]int {
	hits := map[int]int{}
	for line := mount.StartLine + 1; line < mount.EndLine; line++ {
		hits[line] = 1
	}
	for _, line := range bodies {
		hits[line] = 0
	}
	return hits
}

// routesOwnTheirMeasures is whether a branch added to a route mounted in
// mount changes the route's complexity, mutation sites, syntax nodes and
// hash and none of mount's; and whether mount is wholly covered when only
// its routes' bodies never ran, they being uncovered though mount ran.
func (u *unitsState) routesOwnTheirMeasures() error {
	mount, route, bodies, err := u.mounted()
	if err != nil {
		return err
	}
	before, err := u.measure(u.w.dir, u.file)
	if err != nil {
		return err
	}
	// The same file in a folder of its own, a branch added on the route's
	// second line, so no line moves.
	lines := strings.Split(u.source, "\n")
	body := lines[route.StartLine]
	indent := body[:len(body)-len(strings.TrimLeft(body, " "))]
	lines[route.StartLine] = indent + "if (globalThis.a && globalThis.b) { return null; } " + strings.TrimLeft(body, " ")
	changed := filepath.Join(u.w.dir, "changed")
	if err := u.w.writeIn(changed, u.file, strings.Join(lines, "\n")); err != nil {
		return err
	}
	after, err := u.measure(changed, u.file)
	if err != nil {
		return err
	}
	// A unit with no mutation site has none listed; every unit has the
	// other three.
	for _, m := range []measures{before, after} {
		for _, name := range []string{mount.Name, route.Name} {
			m.sites[name] += 0
		}
	}
	for _, c := range []struct {
		what          string
		before, after map[string]string
	}{
		{"complexity", texts(before.complexity), texts(after.complexity)},
		{"mutation sites", texts(before.sites), texts(after.sites)},
		{"syntax nodes", texts(before.nodes), texts(after.nodes)},
		{"hash", before.hash, after.hash},
	} {
		if c.before[mount.Name] == "" || c.before[route.Name] == "" {
			return fmt.Errorf("no %s of mount or %s: %v", c.what, route.Name, c.before)
		}
		if c.before[mount.Name] != c.after[mount.Name] {
			return fmt.Errorf("a branch added to %s changed mount's %s from %s to %s", route.Name, c.what, c.before[mount.Name], c.after[mount.Name])
		}
		if c.before[route.Name] == c.after[route.Name] {
			return fmt.Errorf("a branch added to %s left its %s %s", route.Name, c.what, c.before[route.Name])
		}
	}
	// Coverage: every line of mount ran but the bodies of its routes.
	coverage, err := u.coverage(mountRan(mount, bodies))
	if err != nil {
		return err
	}
	if coverage[mount.Name] != "100" || coverage[route.Name] != "0" {
		return fmt.Errorf("with every line of mount run but its routes' bodies, mount's coverage is %s%% and %s's %s%%, not 100%% and 0%%", coverage[mount.Name], route.Name, coverage[route.Name])
	}
	return nil
}

// routeStartLineIsMounts is whether the line a route mounted in mount
// starts on counts for mount, not the route: run, it leaves the route
// uncovered; not run, mount is not wholly covered.
func (u *unitsState) routeStartLineIsMounts() error {
	mount, route, bodies, err := u.mounted()
	if err != nil {
		return err
	}
	for _, ran := range []int{1, 0} {
		hits := mountRan(mount, bodies)
		hits[route.StartLine] = ran
		coverage, err := u.coverage(hits)
		if err != nil {
			return err
		}
		if ran == 1 && coverage[route.Name] != "0" {
			return fmt.Errorf("with line %d run and its body not, %s's coverage is %s%%, not 0%%", route.StartLine, route.Name, coverage[route.Name])
		}
		if ran == 0 && coverage[mount.Name] == "100" {
			return fmt.Errorf("with line %d not run, mount's coverage is 100%%: the line is not mount's", route.StartLine)
		}
	}
	return nil
}

// coverage is each unit's coverage, in percent, as crap reads it from an
// LCOV report of the scenario's file that ran each line of hits as often as
// it says.
func (u *unitsState) coverage(hits map[int]int) (map[string]string, error) {
	var report strings.Builder
	fmt.Fprintf(&report, "SF:%s\n", u.file)
	lines := make([]int, 0, len(hits))
	for line := range hits {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	for _, line := range lines {
		fmt.Fprintf(&report, "DA:%d,%d\n", line, hits[line])
	}
	report.WriteString("end_of_record\n")
	if err := u.w.write("lcov.info", report.String()); err != nil {
		return nil, err
	}
	var crap struct {
		Entries []struct {
			Name     string
			Coverage *float64
		}
	}
	if err := u.runJSON(u.w.dir, &crap, "crap", "--coverage-report", "lcov.info", "--json", u.file); err != nil {
		return nil, err
	}
	coverage := map[string]string{}
	for _, e := range crap.Entries {
		if e.Coverage != nil {
			coverage[e.Name] = strconv.FormatFloat(*e.Coverage, 'f', -1, 64)
		}
	}
	return coverage, nil
}

// texts is m with each number as its text.
func texts(m map[string]int) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = strconv.Itoa(v)
	}
	return out
}

// routeName is a route unit's name: a method in capitals, alone or before
// a path.
var routeName = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|ALL|USE)( |$)`)

// testFilesHaveNoRoutes is whether the scenario's file, written as a test
// file beside it, has units but no route among them.
func (u *unitsState) testFilesHaveNoRoutes() error {
	ext := filepath.Ext(u.file)
	test := strings.TrimSuffix(u.file, ext) + ".test" + ext
	if err := u.w.write(test, u.source); err != nil {
		return err
	}
	if err := u.w.runWith(u.w.env(), "units --tests --json "+test); err != nil {
		return err
	}
	all, err := u.listed()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("%s, a test file, has no unit at all\n%s", test, u.w.report())
	}
	for _, x := range all {
		if routeName.MatchString(x.Name) {
			return fmt.Errorf("%s, in a test file, is a route unit\n%s", x.id(), u.w.report())
		}
	}
	return nil
}
