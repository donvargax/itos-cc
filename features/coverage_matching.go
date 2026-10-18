package features

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Matching report paths to sources, and the pieces a report lists more than
// once.

// source writes the source rel, slash-separated from the folder the run
// goes from, of the language of its extension, which the run selects.
func (c *coverageState) source(rel string) error {
	text, _, _, err := shapeOf(rel)
	if err != nil {
		return err
	}
	c.sources = append(c.sources, rel)
	return c.w.write(path.Join(c.runDir, rel), text)
}

// under is a path a scenario writes from the root, /p/a.go, as a path of
// the scratch folder, p/a.go.
func under(p string) string { return strings.TrimPrefix(p, "/") }

// exactEntries writes a/x/board.go and b/x/board.go, which share a tail,
// and names the one relative to the folder the run goes from and the
// other by its absolute path.
func (c *coverageState) exactEntries() error {
	for _, s := range []string{"a/x/board.go", "b/x/board.go"} {
		if err := c.source(s); err != nil {
			return err
		}
	}
	c.entries = []string{"a/x/board.go", c.abs("b/x/board.go")}
	return nil
}

// sourceFile writes the source p names from the root.
func (c *coverageState) sourceFile(p string) error { return c.source(under(p)) }

// sourcesUnder writes the sources the paths name from the root, which share
// their first folder: the folder the run goes from, which selects them.
func (c *coverageState) sourcesUnder(paths ...string) error {
	root, _, _ := strings.Cut(under(paths[0]), "/")
	c.runDir = root
	for _, p := range paths {
		rel, ok := strings.CutPrefix(under(p), root+"/")
		if !ok {
			return fmt.Errorf("%s is not in %s with %s", p, root, paths[0])
		}
		if err := c.source(rel); err != nil {
			return err
		}
	}
	if len(paths) == 1 {
		c.paths = c.sources
	}
	return nil
}

// existingEntries names a file by both entries, the one relative to the
// folder the run goes from and the other from the root, and writes it
// there, a source the run does not select.
func (c *coverageState) existingEntries(relative, fromRoot string) error {
	text, _, _, err := shapeOf(relative)
	if err != nil {
		return err
	}
	if path.Join(c.runDir, relative) != under(fromRoot) {
		return fmt.Errorf("%s and %s name different files from %s", relative, fromRoot, c.runDir)
	}
	if err := c.w.write(under(fromRoot), text); err != nil {
		return err
	}
	c.entries = append(c.entries, relative, c.abs(under(fromRoot)))
	for _, e := range []string{c.abs(path.Join(c.runDir, relative)), c.entries[1]} {
		if !c.w.d.Exists(e) {
			return fmt.Errorf("no file is at %s", e)
		}
	}
	return nil
}

// sourceBesideProjectFile writes the source the run selects and a project
// file beside it, in a module whose path the entries name.
func (c *coverageState) sourceBesideProjectFile(source, file string) error {
	if err := c.w.write("go.mod", "module example.com/m\n\ngo 1.22\n"); err != nil {
		return err
	}
	if err := c.source(source); err != nil {
		return err
	}
	c.paths = []string{source}
	text, _, _, err := shapeOf(file)
	if err != nil {
		return err
	}
	return c.w.write(file, text)
}

// symlinkedSource writes real/board.go and links src to real, the run
// selecting src/board.go.
func (c *coverageState) symlinkedSource() error {
	text, _, _, err := shapeOf("real/board.go")
	if err != nil {
		return err
	}
	if err := c.w.write("real/board.go", text); err != nil {
		return err
	}
	if err := c.w.d.Link(c.abs("real"), c.abs("src")); err != nil {
		return err
	}
	c.sources, c.paths = []string{"src/board.go"}, []string{"src/board.go"}
	return nil
}

// entryFromBase names entry, which is no file of the folder the run goes
// from, and neither source is in that folder itself.
func (c *coverageState) entryFromBase(entry string) error {
	if c.w.d.Exists(c.abs(path.Join(c.runDir, entry))) {
		return fmt.Errorf("the base directory holds %s", entry)
	}
	for _, s := range c.sources {
		if !strings.Contains(s, "/") {
			return fmt.Errorf("the base directory holds %s", s)
		}
	}
	c.entries = append(c.entries, entry)
	return nil
}

// separators writes src/demo/board.go, and two reports of it: one naming
// it with written separators, the other with the reverse, each read in a
// run of its own.
func (c *coverageState) separators(written, reverse string) error {
	if err := c.source("src/demo/board.go"); err != nil {
		return err
	}
	parts := []string{"src", "demo", "board.go"}
	c.cases = [][]string{{strings.Join(parts, written)}, {strings.Join(parts, reverse)}}
	return nil
}

// matchToSources writes each case's report, LCOV or a Go profile of its
// entries, each measuring its file as the source of that name is
// measured, and runs crap --json over it from the folder the run goes
// from, naming the paths a Given selected.
func (c *coverageState) matchToSources() error {
	cases := c.cases
	if len(cases) == 0 {
		cases = [][]string{c.entries}
	}
	if len(cases[0]) == 0 || len(c.sources) == 0 {
		return errors.New("no report entry and sources were written to match")
	}
	c.matched = nil
	for i, entries := range cases {
		name := fmt.Sprintf("report-%d.info", i)
		var b strings.Builder
		if c.goFormat {
			name = fmt.Sprintf("report-%d.out", i)
			b.WriteString("mode: set\n")
		}
		for _, e := range entries {
			_, _, pieces, err := shapeOf(filepath.ToSlash(e))
			if err != nil {
				return err
			}
			if c.goFormat {
				b.WriteString(goRecords(e, goBlocks(true, false, true)))
				continue
			}
			b.WriteString(lcovRecord(e, pieces, nil))
		}
		if err := c.w.write(path.Join(c.runDir, name), b.String()); err != nil {
			return err
		}
		if err := c.w.runEnv(c.abs(c.runDir), c.w.env(), append([]string{"crap", "--json", "--coverage-report", name}, c.paths...)...); err != nil {
			return err
		}
		entries, err := c.listed()
		if err != nil {
			return err
		}
		c.matched = append(c.matched, entries)
	}
	return nil
}

// matchedCoverage is what the report gives a source it matches: its shape's
// share, by a Go profile's statements or by LCOV's lines.
func (c *coverageState) matchedCoverage(source string) (float64, error) {
	_, _, pieces, err := shapeOf(source)
	if c.goFormat {
		pieces = goBlocks(true, false, true)
	}
	want, _ := share(pieces, byUnit)
	return want, err
}

// matches is whether, in every case's run, each source selected has the
// coverage its entry gives it.
func (c *coverageState) matches() error {
	if len(c.matched) == 0 {
		return errors.New("nothing was matched")
	}
	for _, entries := range c.matched {
		for _, s := range c.sources {
			_, name, _, err := shapeOf(s)
			if err != nil {
				return err
			}
			want, err := c.matchedCoverage(s)
			if err != nil {
				return err
			}
			if err := c.hasCoverage(entries, covFunction{s, name}, want); err != nil {
				return err
			}
		}
	}
	return nil
}

// matchesNeither is whether each source selected is listed with no
// coverage.
func (c *coverageState) matchesNeither() error {
	if len(c.matched) == 0 {
		return errors.New("nothing was matched")
	}
	for _, entries := range c.matched {
		for _, s := range c.sources {
			_, name, _, err := shapeOf(s)
			if err != nil {
				return err
			}
			got, err := coverageOf(entries, covFunction{s, name})
			if err != nil {
				return fmt.Errorf("%v\n%s", err, c.w.report())
			}
			if got != nil {
				return fmt.Errorf("%s of %s has %s%% coverage: an entry matched it\n%s", name, s, shown(got), c.w.report())
			}
		}
	}
	return nil
}

// ignored are the entries of COV-33's report that match no selected
// source: a test file beside the source and a dependency's file.
var ignored = []string{"board_test.go", "vendor/example.com/dep/dep.go"}

// unmatchedEntry writes board.go and its test, and a report of board.go
// and of the files ignored names.
func (c *coverageState) unmatchedEntry() error {
	if err := c.w.write("board_test.go", markerTest("board", 5)); err != nil {
		return err
	}
	f := covFixture{format: "LCOV", report: "lcov.info", source: "board.go", function: "Place", entry: "board.go", unit: "line", pieces: goLines}
	report := lcovRecord(f.entry, f.pieces, nil)
	for _, e := range ignored {
		report += lcovRecord(e, lcovLines([2]int{4, 1}, [2]int{5, 1}), nil)
	}
	return c.add(f, goSource("board"), report)
}

// entryIgnored is whether the run succeeded, listing board.go's functions
// alone with their coverage, and said nothing of the ignored entries.
func (c *coverageState) entryIgnored() error {
	f, err := c.fixture()
	if err != nil {
		return err
	}
	object, report, err := (&crapState{w: c.w}).lastJSON()
	if err != nil {
		return err
	}
	if c.w.exit != 0 || len(object.Problems) > 0 || len(object.Unmeasured) > 0 {
		return fmt.Errorf("the run did not ignore the entries\n%s", report)
	}
	for _, e := range object.Entries {
		if filepath.ToSlash(e.File) != f.source {
			return fmt.Errorf("%s of %s is listed\n%s", e.Name, e.File, report)
		}
	}
	for _, e := range ignored {
		if strings.Contains(filepath.ToSlash(c.w.stderr), e) {
			return fmt.Errorf("stderr names %s\n%s", e, report)
		}
	}
	want, _ := share(f.pieces, byUnit)
	return c.hasCoverage(object.Entries, covFunction{f.source, f.function}, want)
}

// Pieces listed more than once.

// threePackages writes the module example.com/m with the packages named,
// each of goSource's Place and a test of it.
func (c *coverageState) threePackages(a, b, cc string) error {
	files := map[string]string{"go.mod": "module example.com/m\n\ngo 1.22\n"}
	for _, p := range []string{a, b, cc} {
		files[p+"/"+p+".go"], files[p+"/"+p+"_test.go"] = goSource(p), markerTest(p, 5)
		c.fixtures = append(c.fixtures, covFixture{format: "Go cover profile", report: "cover.out", source: p + "/" + p + ".go", function: "Place",
			entry: "example.com/m/" + p + "/" + p + ".go", unit: "statement", pieces: goBlocks(true, true, true)})
	}
	return c.writeIn(".", files)
}

// everyBinaryProfile writes cover.out as go test -coverpkg=./... writes it
// for the packages: each test binary lists every block of every package,
// covered in its own package alone.
func (c *coverageState) everyBinaryProfile() error {
	if len(c.fixtures) == 0 {
		return errors.New("no packages were written")
	}
	var b strings.Builder
	b.WriteString("mode: set\n")
	for _, binary := range c.fixtures {
		for _, f := range c.fixtures {
			b.WriteString(goRecords(f.entry, goBlocks(f.entry == binary.entry, f.entry == binary.entry, f.entry == binary.entry)))
		}
	}
	return c.w.write("cover.out", b.String())
}

// goToolCover is a function's coverage as go tool cover -func gives it from
// profile: its blocks, each once, covered when any copy of it ran.
func goToolCover(profile, entry string) float64 {
	covered := map[string]bool{}
	statements := map[string]float64{}
	for _, line := range strings.Split(profile, "\n") {
		block, ok := strings.CutPrefix(line, entry+":")
		if !ok {
			continue
		}
		fields := strings.Fields(block)
		n, _ := strconv.ParseFloat(fields[1], 64)
		statements[fields[0]] = n
		covered[fields[0]] = covered[fields[0]] || fields[2] != "0"
	}
	var all, ran float64
	for span, n := range statements {
		all += n
		if covered[span] {
			ran += n
		}
	}
	if all == 0 {
		return 0
	}
	return 100 * ran / all
}

// asGoToolCover is whether the function scored has percent coverage, what
// go tool cover -func gives it from the profile.
func (c *coverageState) asGoToolCover(percent int) error {
	if len(c.fixtures) == 0 {
		return errors.New("no profile was written")
	}
	f := c.fixtures[0]
	data, err := c.w.readIn(c.w.dir, "cover.out")
	if err != nil {
		return err
	}
	if tool := goToolCover(string(data), f.entry); tool != float64(percent) {
		return fmt.Errorf("go tool cover -func gives %s %v%%, not %d%%", f.function, tool, percent)
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{f.source, f.function}, float64(percent))
}

// splitSource is board.go of F, of no decision, on lines 3 to 7, and G,
// whose switch on line 10 decides between three branches.
const splitSource = "package board\n\nfunc F(a int) int {\n\tn := a\n\tn++\n\treturn n\n}\n\n" +
	"func G(a int) int {\n\tswitch {\n\tcase a > 1:\n\t\treturn 1\n\tcase a < -1:\n\t\treturn 2\n\t}\n\treturn 3\n}\n"

// twoReports writes board.go, which two reports are to measure.
func (c *coverageState) twoReports() error {
	c.split = &covSplit{reports: [2]string{"unit.info", "integration.info"}}
	return c.w.write("board.go", splitSource)
}

// eachCoversALine writes the two reports: each runs a line of F the other
// does not, and takes branches of G's decision the other does not, the
// second more of them.
func (c *coverageState) eachCoversALine() error {
	s := c.split
	if s == nil {
		return errors.New("no two reports were named")
	}
	s.f = [2][]covPiece{
		lcovLines([2]int{4, 1}, [2]int{5, 1}, [2]int{6, 0}),
		lcovLines([2]int{4, 1}, [2]int{5, 0}, [2]int{6, 1}),
	}
	s.g = [2]covDecision{{line: 10, branches: 3, taken: []int{0}}, {line: 10, branches: 3, taken: []int{1, 2}}}
	for i, name := range s.reports {
		gLines := lcovLines([2]int{10, 1}, [2]int{11, 1}, [2]int{12, 1 - i}, [2]int{13, i}, [2]int{14, i}, [2]int{16, 0})
		if err := c.w.write(name, lcovRecord("board.go", append(slices.Clone(s.f[i]), gLines...), []covDecision{s.g[i]})); err != nil {
			return err
		}
	}
	return nil
}

// anyReportCovers is whether F has the share of its lines either report
// ran, more than either alone ran.
func (c *coverageState) anyReportCovers() error {
	s := c.split
	if s == nil || s.f[0] == nil {
		return errors.New("no two reports were written")
	}
	union := slices.Clone(s.f[0])
	for i, p := range s.f[1] {
		union[i].covered = max(union[i].covered, p.covered)
	}
	want, _ := share(union, byUnit)
	for _, alone := range s.f {
		if got, _ := share(alone, byUnit); got >= want {
			return errors.New("one report alone covers every line of F the two do")
		}
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"board.go", "F"}, want)
}

// mostBranches is whether G has the share of its decision's branches the
// report that took most of them took, less than the two took together.
func (c *coverageState) mostBranches() error {
	s := c.split
	if s == nil || s.f[0] == nil {
		return errors.New("no two reports were written")
	}
	most, both := 0, map[int]bool{}
	for _, d := range s.g {
		most = max(most, len(d.taken))
		for _, b := range d.taken {
			both[b] = true
		}
	}
	if most == len(both) {
		return errors.New("one report took every branch the two took")
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"board.go", "G"}, 100*float64(most)/float64(s.g[0].branches))
}
