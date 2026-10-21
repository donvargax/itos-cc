package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Then steps of the coverage commands run.

// runs is whether a coverage command of the last run is pattern, as the
// scenario writes it, with no word beginning without when that is not "".
func (c *coverageState) runs(pattern, without string) error {
	re := commandPattern(pattern)
	for _, a := range c.announced(c.w.stderr) {
		if !re.MatchString(a.command) {
			continue
		}
		if without != "" && slices.ContainsFunc(a.words, func(w string) bool { return strings.HasPrefix(w, without) }) {
			return fmt.Errorf("coverage runs %q, with %s\n%s", a.command, without, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("no coverage command is %q\n%s", pattern, c.w.report())
}

// sameAsWholeModule is whether the functions of a/a.go, of the coverage
// the last run gave them, have the same in a run of the whole module's
// tests, which covers them.
func (c *coverageState) sameAsWholeModule() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--json", "--all-tests", "a/a.go")
	if err != nil {
		return err
	}
	report := fmt.Sprintf("%s\n--- with --all-tests, exit %d\n--- stdout\n%s--- stderr\n%s", c.w.report(), code, stdout, stderr)
	whole := false
	for _, a := range c.announced(stderr) {
		whole = whole || slices.Equal(a.words[len(a.words)-2:], []string{"-coverpkg=./...", "./..."})
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 || !whole {
		return fmt.Errorf("the whole module's tests did not measure a/a.go\n%s", report)
	}
	n := 0
	for _, e := range object.Entries {
		got, err := coverageOf(entries, covFunction{filepath.ToSlash(e.File), e.Name})
		switch {
		case err != nil:
			return fmt.Errorf("%v\n%s", err, report)
		case e.Coverage == nil:
			return fmt.Errorf("the whole module's tests do not measure %s\n%s", e.id(), report)
		case shown(got) != shown(e.Coverage):
			return fmt.Errorf("%s has %s%% coverage, and %s%% with the whole module's tests\n%s", e.id(), shown(got), shown(e.Coverage), report)
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("no function of a/a.go is scored\n%s", report)
	}
	return nil
}

// runsEither is whether a coverage command of the last run is one tool's
// with those words, or the other's: its program is the tool, and the words
// follow it in a row.
func (c *coverageState) runsEither(tool, words, other, otherWords string) error {
	for _, a := range c.announced(c.w.stderr) {
		for _, want := range [][2]string{{tool, words}, {other, otherWords}} {
			if program(a.words[0]) == want[0] && strings.Contains(strings.Join(a.words[1:], " ")+" ", want[1]+" ") {
				return nil
			}
		}
	}
	return fmt.Errorf("coverage runs neither %s %s nor %s %s\n%s", tool, words, other, otherWords, c.w.report())
}

// selectedRoots are the build roots of the sources the last run named, or
// every root when it named none.
func (c *coverageState) selectedRoots() ([]string, error) {
	if c.w.last == nil {
		return nil, errors.New("no run of itos-cc to read")
	}
	var paths []string
	for _, a := range c.w.last.args {
		if !strings.HasPrefix(a, "-") && !slices.Contains([]string{"crap", "mutation", "run"}, a) {
			paths = append(paths, filepath.ToSlash(a))
		}
	}
	var roots []string
	for root := range c.roots {
		if len(paths) == 0 {
			roots = append(roots, root)
		}
	}
	for _, p := range paths {
		best := "."
		for root := range c.roots {
			if root != "." && strings.HasPrefix(p, root+"/") && len(root) > len(best) {
				best = root
			}
		}
		if !slices.Contains(roots, best) {
			roots = append(roots, best)
		}
	}
	slices.Sort(roots)
	return roots, nil
}

// wholeSuite is whether the last run's coverage commands are one for each
// build root of the sources it named, each running that root's every
// package: go test over ./....
func (c *coverageState) wholeSuite() error {
	roots, err := c.selectedRoots()
	if err != nil {
		return err
	}
	var dirs []string
	for _, a := range c.announced(c.w.stderr) {
		if c.roots[a.dir] != "go" || !slices.Equal(a.words[:2], []string{"go", "test"}) {
			return fmt.Errorf("no whole suite is known for %q in %s\n%s", a.command, a.dir, c.w.report())
		}
		if !slices.Equal(a.words[len(a.words)-2:], []string{"-coverpkg=./...", "./..."}) {
			return fmt.Errorf("coverage in %s runs %q, not every package\n%s", a.dir, a.command, c.w.report())
		}
		dirs = append(dirs, a.dir)
	}
	slices.Sort(dirs)
	if !slices.Equal(dirs, roots) {
		return fmt.Errorf("coverage runs in %v, not once in each build root of %v\n%s", dirs, roots, c.w.report())
	}
	return nil
}

// commands are the coverage commands of the last run announced for the
// project's folder.
func (c *coverageState) commands() []covAnnouncement {
	var out []covAnnouncement
	for _, a := range c.announced(c.w.stderr) {
		if a.dir == c.project {
			out = append(out, a)
		}
	}
	return out
}

// commandIs is whether the project's coverage commands are what spec says:
// commands joined by ", then ", each held by a command after the one
// holding the one before. A command holds every word of its spec, <pm>
// standing for the project's package manager, but where it names the
// installed tool, which its program must be, from a node_modules/.bin: then
// run is the tool's next word, "with the v8 provider" Vitest's own
// announcement of it, around the words it ends with, and the rest words it
// holds. Writing LCOV, it asks for an LCOV reporter; read from a path, that
// report is there after the run and gives each function what the
// project's script wrote in it.
func (c *coverageState) commandIs(spec string) error {
	commands := c.commands()
	if len(commands) == 0 {
		return fmt.Errorf("no coverage command runs for the project\n%s", c.w.report())
	}
	next := 0
	for _, part := range strings.Split(spec, ", then ") {
		part, readFrom, _ := strings.Cut(part, ", read from ")
		part, lcov := strings.CutSuffix(strings.TrimSuffix(part, ","), " writing LCOV")
		part = strings.TrimSuffix(strings.ReplaceAll(part, "<pm>", c.pm), ",")
		i := slices.IndexFunc(commands[next:], func(a covAnnouncement) bool { return c.holds(a, part, lcov) == nil })
		if i < 0 {
			reason := errors.New("no command")
			if next < len(commands) {
				reason = c.holds(commands[next], part, lcov)
			}
			return fmt.Errorf("no coverage command is %s: %v\n%s", part, reason, c.w.report())
		}
		next += i + 1
		if readFrom != "" {
			if err := c.readFromReport(readFrom); err != nil {
				return err
			}
		}
	}
	return nil
}

// holds is why a is not the command spec says, nil when it is.
func (c *coverageState) holds(a covAnnouncement, spec string, lcov bool) error {
	words := a.words
	if lcov && !slices.ContainsFunc(words, func(w string) bool { return regexp.MustCompile(`[Rr]eporters?=lcov$`).MatchString(w) }) && !c.wroteLCOV(words) {
		return fmt.Errorf("%q asks for no LCOV reporter, and writes no LCOV where its --coverage-path names", a.command)
	}
	if rest, ok := strings.CutPrefix(spec, "the installed "); ok {
		tool, rest, _ := strings.Cut(rest, " ")
		bin, inModules := c.rel(path.Dir(filepath.ToSlash(words[0])))
		if program(words[0]) != tool || !inModules || path.Base(bin) != ".bin" || path.Base(path.Dir(bin)) != "node_modules" {
			return fmt.Errorf("%q does not run the installed %s", a.command, tool)
		}
		words = words[1:]
		if after, ok := strings.CutPrefix(rest, "run "); ok {
			if len(words) == 0 || words[0] != "run" {
				return fmt.Errorf("%q does not run %s run", a.command, tool)
			}
			rest = after
		}
		if provider, ok := strings.CutPrefix(rest, "with the "); ok {
			provider = strings.TrimSuffix(provider, " provider")
			// Vitest colours its words where the environment asks for it.
			plain := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(c.w.stderr, "")
			if !strings.Contains(plain, "Coverage enabled with "+provider) {
				return fmt.Errorf("%s does not say coverage is enabled with %s", tool, provider)
			}
			return nil
		}
		if around, ok := strings.CutPrefix(rest, "around "); ok {
			tail := strings.Fields(around)
			if len(words) < len(tail) || !slices.Equal(words[len(words)-len(tail):], tail) {
				return fmt.Errorf("%q does not run around %s", a.command, around)
			}
			return nil
		}
		spec = rest
	}
	for _, want := range strings.Fields(spec) {
		re := wordPattern(want)
		if !slices.ContainsFunc(words, re.MatchString) {
			return fmt.Errorf("%q does not hold %s", a.command, want)
		}
	}
	return nil
}

// wroteLCOV is whether a command of words, such as flutter test's, which
// has no reporter to ask for and writes LCOV alone, wrote an LCOV record
// where its --coverage-path names, a file of the scratch folder's run
// folder under .metrics/coverage/: once the run ends, its report's folder
// is kept directly under .metrics/coverage/, for --use-existing-coverage,
// and the run folder is gone.
func (c *coverageState) wroteLCOV(words []string) bool {
	for _, w := range words {
		report, ok := strings.CutPrefix(w, "--coverage-path=")
		if !ok {
			continue
		}
		rel, in := c.rel(report)
		if !in || !strings.HasPrefix(rel, ".metrics/coverage/") {
			return false
		}
		data, err := c.w.d.Read(c.abs(path.Join(".metrics/coverage", path.Base(path.Dir(rel)), path.Base(rel))))
		return err == nil && strings.HasPrefix(string(data), "SF:") && strings.Contains(string(data), "end_of_record")
	}
	return false
}

// readFromReport is whether the report the project's script wrote is at
// file, of the project's folder, and each function has the coverage the
// script wrote in it.
func (c *coverageState) readFromReport(file string) error {
	if !c.w.d.Exists(c.abs(path.Join(c.project, file))) {
		return fmt.Errorf("no report is at %s\n%s", file, c.w.report())
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(c.want) == 0 {
		return errors.New("the project's script wrote no report")
	}
	for f, want := range c.want {
		if err := c.hasCoverage(entries, f, want); err != nil {
			return err
		}
	}
	return nil
}

// pmIs is whether the project's coverage script runs with manager: the
// command that runs it is manager's.
func (c *coverageState) pmIs(manager string) error {
	for _, a := range c.commands() {
		if len(a.words) >= 3 && slices.Equal(a.words[1:3], []string{"run", "coverage"}) {
			if program(a.words[0]) != manager {
				return fmt.Errorf("the project's coverage script runs with %s, not %s\n%s", program(a.words[0]), manager, c.w.report())
			}
			return nil
		}
	}
	return fmt.Errorf("no command runs the project's coverage script\n%s", c.w.report())
}

// nearestNodeBin is whether the project's coverage command is Vitest's,
// Jest's or c8's from the node_modules/.bin nearest the project that
// holds it.
func (c *coverageState) nearestNodeBin() error {
	for _, a := range c.commands() {
		tool := program(a.words[0])
		if !slices.Contains([]string{"vitest", "jest", "c8"}, tool) {
			continue
		}
		nearest := ""
		for dir := c.project; nearest == ""; dir = path.Dir(dir) {
			if bin := path.Join(dir, "node_modules/.bin"); c.w.d.Exists(c.abs(path.Join(bin, tool))) {
				nearest = bin
			}
			if dir == "." {
				break
			}
		}
		got, _ := c.rel(path.Dir(filepath.ToSlash(a.words[0])))
		if nearest == "" || got != nearest {
			return fmt.Errorf("%s runs from %s, not the nearest node_modules/.bin, %s\n%s", tool, got, nearest, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("neither vitest, jest nor c8 runs for the project\n%s", c.w.report())
}

// noNpx is whether no coverage command runs npx.
func (c *coverageState) noNpx() error {
	announced := c.announced(c.w.stderr)
	if len(announced) == 0 {
		return fmt.Errorf("no coverage command runs\n%s", c.w.report())
	}
	for _, a := range announced {
		if slices.ContainsFunc(a.words, func(w string) bool { return program(w) == "npx" }) {
			return fmt.Errorf("%q runs npx\n%s", a.command, c.w.report())
		}
	}
	return nil
}

// nothingRuns is whether no coverage command runs for the project.
func (c *coverageState) nothingRuns() error {
	if commands := c.commands(); len(commands) > 0 {
		return fmt.Errorf("coverage runs %q for the project\n%s", commands[0].command, c.w.report())
	}
	return nil
}

// saysWhatToInstall is whether stderr says, of the project's folder, that
// the tool the Given left out is missing: a line naming the folder and the
// tool that says to install or add it, or to measure with
// --coverage-command.
func (c *coverageState) saysWhatToInstall() error {
	if c.missing == "" {
		return errors.New("no Given left a tool out")
	}
	for _, line := range strings.Split(c.w.stderr, "\n") {
		rest, ok := strings.CutPrefix(line, "itos-cc: coverage: ")
		if !ok {
			continue
		}
		dir, reason, ok := strings.Cut(rest, ": ")
		if rel, in := c.rel(dir); !ok || !in || rel != c.project {
			continue
		}
		says := strings.Contains(reason, "install") || strings.Contains(reason, "add ") || strings.Contains(reason, "--coverage-command")
		if strings.Contains(strings.ToLower(reason), strings.ToLower(c.missing)) && says {
			return nil
		}
	}
	return fmt.Errorf("stderr does not say to install %s for the project, or to use --coverage-command\n%s", c.missing, c.w.report())
}

// onlyComplexity is whether the last run lists functions, of the project's
// folder for it, each with a complexity and neither coverage nor a score.
func (c *coverageState) onlyComplexity(forIt string) error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if forIt != "" && c.project != "." && !strings.HasPrefix(filepath.ToSlash(e.File), c.project+"/") {
			continue
		}
		n++
		if e.Complexity < 1 || e.Coverage != nil || e.CRAP != nil {
			return fmt.Errorf("%s has complexity %d, coverage %s and CRAP %s\n%s", e.id(), e.Complexity, shown(e.Coverage), shown(e.CRAP), c.w.report())
		}
	}
	if n == 0 {
		return fmt.Errorf("no function is listed\n%s", c.w.report())
	}
	return nil
}

// venvRunsTests is whether the project's .venv's python runs its tests,
// which measure place.
func (c *coverageState) venvRunsTests() error {
	for _, a := range c.commands() {
		python, _ := c.rel(a.words[0])
		i := slices.Index(a.words, "-m")
		if python != ".venv/bin/python" || !slices.ContainsFunc(a.words, func(w string) bool { return w == "pytest" || w == "unittest" }) || i < 0 {
			continue
		}
		entries, err := c.listed()
		if err != nil {
			return err
		}
		if got, err := coverageOf(entries, covFunction{"board.py", "place"}); err != nil || got == nil {
			return fmt.Errorf("the tests the .venv's python ran do not measure place\n%s", c.w.report())
		}
		return nil
	}
	return fmt.Errorf("the .venv's python runs no tests\n%s", c.w.report())
}

// wrapperUsed is whether the module's coverage command is the build root's
// gradlew, and no coverage command is gradle.
func (c *coverageState) wrapperUsed() error {
	commands := c.commands()
	if len(commands) == 0 {
		return fmt.Errorf("no coverage command runs for the module\n%s", c.w.report())
	}
	for _, a := range c.announced(c.w.stderr) {
		if program(a.words[0]) == "gradle" {
			return fmt.Errorf("coverage runs %q\n%s", a.command, c.w.report())
		}
	}
	if wrapper, _ := c.rel(commands[0].words[0]); wrapper != "gradlew" {
		return fmt.Errorf("the module's coverage runs %s, not the build root's gradlew\n%s", commands[0].words[0], c.w.report())
	}
	return nil
}

// toolPrograms are the programs a language's coverage command may start
// with.
var toolPrograms = map[string][]string{"go": {"go"}, "typescript": {"vitest", "jest", "c8", "npm", "pnpm", "yarn", "bun"}}

// eachRootRuns is whether each build root's coverage command is its
// language's, run in that root.
func (c *coverageState) eachRootRuns() error {
	for root, language := range c.roots {
		i := slices.IndexFunc(c.announced(c.w.stderr), func(a covAnnouncement) bool { return a.dir == root })
		if i < 0 {
			return fmt.Errorf("no coverage command runs in %s\n%s", root, c.w.report())
		}
		if a := c.announced(c.w.stderr)[i]; !slices.Contains(toolPrograms[language], program(a.words[0])) {
			return fmt.Errorf("%s runs %q, no %s coverage command\n%s", root, a.command, language, c.w.report())
		}
	}
	return nil
}

// mergedIntoOne is whether the one run gave a function of each build root
// coverage.
func (c *coverageState) mergedIntoOne() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	for root := range c.roots {
		if !slices.ContainsFunc(entries, func(e crapEntry) bool {
			return strings.HasPrefix(filepath.ToSlash(e.File), root+"/") && e.Coverage != nil
		}) {
			return fmt.Errorf("no function of %s has coverage\n%s", root, c.w.report())
		}
	}
	return nil
}

// failureLogged is whether stderr shows the test failing and itos-cc
// saying the coverage command failed with its exit status.
func (c *coverageState) failureLogged() error {
	if !strings.Contains(c.w.stderr, "--- FAIL: TestPlace") ||
		!regexp.MustCompile(`(?m)^itos-cc: coverage: go: exit status [1-9]\d*$`).MatchString(c.w.stderr) {
		return fmt.Errorf("stderr does not log the failing tests\n%s", c.w.report())
	}
	return nil
}

// reportStillUsed is whether Place has the coverage the failing test gave
// it: every statement but its if's arm.
func (c *coverageState) reportStillUsed() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	want, _ := share(goBlocks(true, false, true), byUnit)
	return c.hasCoverage(entries, covFunction{"board.go", "Place"}, want)
}

// reportsUnderMetrics is whether each coverage command of the last run, one
// at least, writes its report under .metrics/coverage/.
func (c *coverageState) reportsUnderMetrics() error {
	announced := c.announced(c.w.stderr)
	if len(announced) == 0 {
		return fmt.Errorf("no coverage command runs\n%s", c.w.report())
	}
	for _, a := range announced {
		i := slices.IndexFunc(a.words, func(w string) bool { return strings.HasPrefix(w, "-coverprofile=") })
		if i < 0 {
			return fmt.Errorf("%q names no report it writes\n%s", a.command, c.w.report())
		}
		report, in := c.rel(strings.TrimPrefix(a.words[i], "-coverprofile="))
		if !in || !strings.HasPrefix(report, ".metrics/coverage/") {
			return fmt.Errorf("%q writes its report outside .metrics/coverage/\n%s", a.command, c.w.report())
		}
	}
	return nil
}

// gitignoreIgnoresAll is whether git, in a repository of the scratch
// folder, ignores whatever is in .metrics/coverage/, its .gitignore
// included, and nothing beside it in .metrics.
func (c *coverageState) gitignoreIgnoresAll() error {
	if err := c.w.git("init"); err != nil {
		return err
	}
	for _, p := range []string{".metrics/coverage/.gitignore", ".metrics/coverage/go-1/coverage.out", ".metrics/coverage/run-1/lcov.info"} {
		if err := c.w.git("check-ignore", "-q", p); err != nil {
			return fmt.Errorf("git does not ignore %s: %v", p, err)
		}
	}
	if c.w.git("check-ignore", "-q", ".metrics/crap.json") == nil {
		return errors.New("git ignores .metrics/crap.json, which is not in .metrics/coverage/")
	}
	return nil
}

// noTestsRun is whether the last run announced no coverage command and no
// test of the project left its mark.
func (c *coverageState) noTestsRun() error {
	if announced := c.announced(c.w.stderr); len(announced) > 0 {
		return fmt.Errorf("coverage runs %q\n%s", announced[0].command, c.w.report())
	}
	if len(c.markers) == 0 {
		return errors.New("the project has no tests that would leave a mark")
	}
	for _, m := range c.markers {
		if c.w.d.Exists(c.abs(m)) {
			return fmt.Errorf("a test ran: %s is there\n%s", m, c.w.report())
		}
	}
	return nil
}

// firstExistingReport is whether each build root's function has the
// coverage of the first of its reports itos-cc looks for that is there.
func (c *coverageState) firstExistingReport() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	if len(c.want) < 2 {
		return errors.New("no project of build roots with reports was written")
	}
	for f, want := range c.want {
		if err := c.hasCoverage(entries, f, want); err != nil {
			return err
		}
	}
	return nil
}

// bothReportsRead is whether each --coverage-report of the last run, named
// relative to the folder it ran in and there, gave the function it measures
// its coverage.
func (c *coverageState) bothReportsRead() error {
	entries, err := c.listed()
	if err != nil {
		return err
	}
	byReport := map[string]covFunction{"a/lcov.info": {"a/a.go", "A"}, "b/coverage.out": {"b/b.go", "B"}}
	args := c.w.last.args
	n := 0
	for i, a := range args {
		if a != "--coverage-report" || i+1 == len(args) {
			continue
		}
		report := args[i+1]
		f, ok := byReport[filepath.ToSlash(report)]
		switch {
		case filepath.IsAbs(report) || !c.w.d.Exists(filepath.Join(c.w.last.dir, report)):
			return fmt.Errorf("%s is not a report relative to the working directory", report)
		case !ok:
			return fmt.Errorf("the sample holds no report %s", report)
		}
		if err := c.hasCoverage(entries, f, c.want[f]); err != nil {
			return err
		}
		n++
	}
	if n < 2 {
		return fmt.Errorf("the run read %d reports, not both\n%s", n, c.w.report())
	}
	return nil
}

// runsThroughShell is whether itos-cc announced command as a shell command
// line, and the shell ran it: make echoed the cover target's recipe.
func (c *coverageState) runsThroughShell(command string) error {
	if !slices.Contains(strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n"), "itos-cc: coverage $ "+command) {
		return fmt.Errorf("itos-cc does not announce %q as a shell command\n%s", command, c.w.report())
	}
	if !strings.Contains(c.w.stderr, sampleRecipe) {
		return fmt.Errorf("%q did not run the Makefile's recipe\n%s", command, c.w.report())
	}
	return nil
}

// readAfterwards is whether report, which the sample did not hold, is
// there after the run, and gave a/a.go's A its coverage.
func (c *coverageState) readAfterwards(report string) error {
	if report != "cover.out" {
		return fmt.Errorf("the sample does not leave %s to be written", report)
	}
	if !c.w.d.Exists(c.abs(report)) {
		return fmt.Errorf("no %s was written\n%s", report, c.w.report())
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{"a/a.go", "A"}, 100*2.0/3)
}
