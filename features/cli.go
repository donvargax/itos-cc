// The steps of cli.feature: the command line every command shares, its
// help, version, flags, exit codes and --json, and the mutation group. Like
// every step here, they read the binary's output alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// cliState is one cli.feature scenario's state beside the world's.
type cliState struct {
	w *world

	// built is what the Given's build made; next is the version a
	// checkout's build is to say it comes before, without its v.
	built Built
	next  string
	// situations are the commands CLI-20's Given set up, each run by the
	// When in a folder of its own.
	situations []situation
}

// situation is a command of CLI-20, by the words of its sentence that
// name it (layOut).
type situation struct{ what string }

// knownSituations are the words layOut knows a command of.
var knownSituations = []string{
	"it succeeded", "a mutant survived", "a function is over a threshold", "a bad flag", "path", "report",
	"a tool", "a report", "a git repository", "an error itos-cc could not classify",
	"a temporary failure, such as serve's port being in use", "an unknown command",
}

func cliSteps(sc *godog.ScenarioContext, w *world) {
	c := &cliState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*c = cliState{w: w}
		if filepath.Base(s.Uri) != "cli.feature" {
			return ctx, nil
		}
		for _, tag := range s.Tags {
			if setUp := cliSamples[tag.Name]; setUp != nil {
				return ctx, setUp(c)
			}
		}
		return ctx, nil
	})

	// Help and version.
	sc.Step(`^the usage text is printed to stdout$`, c.usagePrinted)
	sc.Step(`^it lists the commands (.+)$`, c.listsCommands)
	sc.Step(`^the command's usage, options, --json shape, problem rules, and exit codes are printed to stdout$`, c.commandHelp)
	sc.Step(`^it ends with examples and the address for issue reports$`, c.endsWithExamples)
	sc.Step(`^the binary was built (.+)$`, c.builtAs)
	sc.Step(`^the first line of stdout is "([^"]*)"$`, c.firstLineIs)

	// Flags.
	sc.Step(`^it is the same as "itos-cc ([^"]*)"$`, c.sameAs)
	sc.Step(`^"([^"]*)" is a path, not the flag$`, c.isAPath)

	// The contract with scripts.
	sc.Step(`^(it succeeded|a check said no: .+|a usage or config error: .+|an unknown command|the environment lacks something: .+|an error itos-cc could not classify|a temporary failure, such as serve's port being in use)$`, c.situation)
	sc.Step(`^the command ends$`, c.commandEnds)
	sc.Step(`^stdout is one object with "schema": 1, "ok": true, and the command's own keys$`, c.ownKeys)
	sc.Step(`^progress and test output go to stderr$`, c.progressOnStderr)
	sc.Step(`^stdout is:$`, c.stdoutIs)
	sc.Step(`^a run has a problem$`, func() error { return w.runWith(w.env(), "crap nosuch") })
	sc.Step(`^a Go function scoring 20 beside TypeScript whose coverage tool is not installed$`, c.scoringBesideUnmeasured)

	// The mutation group.
	sc.Step(`^stdout names the subcommands (\w+) and (\w+)$`, c.stdoutNames)
	sc.Step(`^stderr names the subcommands (\w+) and (\w+)$`, c.stderrNames)
	sc.Step(`^stdout says "([^"]*)"$`, w.stdoutSays)
	sc.Step(`^each survivor is a "([^"]*)" problem$`, c.eachSurvivor)
	sc.Step(`^stdout lists each mutation site of (\S+) with its line, column, function, original, and replacement$`, c.listsSites)
	sc.Step(`^no test command runs$`, c.noTestRuns)
}

// Help and version.

// usagePrinted is whether stdout is the top-level help: its usage line,
// then its commands.
func (c *cliState) usagePrinted() error {
	if !strings.HasPrefix(c.w.stdout, "usage: itos-cc ") || !slices.Contains(outputLines(c.w.stdout), "Commands:") {
		return fmt.Errorf("stdout is not the usage text\n%s", c.w.report())
	}
	return nil
}

// listsCommands is whether the help's Commands section lists each command
// of the list, each line's first word naming one.
func (c *cliState) listsCommands(list string) error {
	var listed []string
	in := false
	for _, line := range outputLines(c.w.stdout) {
		switch {
		case line == "Commands:":
			in = true
		case in && strings.TrimSpace(line) == "":
			in = false
		case in:
			if fields := strings.Fields(line); len(fields) > 0 {
				listed = append(listed, fields[0])
			}
		}
	}
	names := splitList(list)
	if len(names) < 2 {
		return fmt.Errorf("%q names fewer than two commands", list)
	}
	for _, name := range names {
		if !slices.Contains(listed, name) {
			return fmt.Errorf("the help lists the commands %v, not %s\n%s", listed, name, c.w.report())
		}
	}
	return nil
}

// commandHelp is whether stdout is the help of the command the run named:
// its usage line, naming the command its words name, then its options, its
// --json shape, its problem rules and its exit codes, in that order.
func (c *cliState) commandHelp() error {
	lines := outputLines(c.w.stdout)
	m := regexp.MustCompile(`^usage: itos-cc (.+?) \[options\]`).FindStringSubmatch(lines[0])
	if m == nil {
		return fmt.Errorf("stdout begins with no command's usage\n%s", c.w.report())
	}
	var words []string
	for _, a := range c.w.last.args {
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
		}
	}
	if len(words) > 0 && words[0] == "help" {
		words = words[1:]
	}
	named := strings.Fields(m[1])
	if len(words) < len(named) || !slices.Equal(words[:len(named)], named) {
		return fmt.Errorf("the usage names %s, not the command of %v\n%s", m[1], c.w.last.args, c.w.report())
	}
	at := 0
	for _, section := range []string{"Options:", "Output with --json", "Problem rules:", "Exit codes:"} {
		i := slices.IndexFunc(lines[at:], func(l string) bool { return strings.HasPrefix(l, section) })
		if i < 0 {
			return fmt.Errorf("no %q after line %d of the help\n%s", section, at+1, c.w.report())
		}
		at += i + 1
	}
	return c.w.exitsWith(0)
}

// endsWithExamples is whether the help's examples follow its exit codes,
// and its last line gives the address for issue reports.
func (c *cliState) endsWithExamples() error {
	lines := outputLines(c.w.stdout)
	exits, examples := slices.Index(lines, "Exit codes:"), slices.Index(lines, "Examples:")
	if exits < 0 || examples < exits {
		return fmt.Errorf("no examples after the exit codes\n%s", c.w.report())
	}
	if last := lines[len(lines)-1]; !regexp.MustCompile(`^Report issues at https://\S+$`).MatchString(last) {
		return fmt.Errorf("the help ends with %q, not the address for issue reports\n%s", last, c.w.report())
	}
	return nil
}

// The builds a Given names, and the release a checkout's build comes
// after.
var (
	releaseBuild  = regexp.MustCompile(`^by a release with -X main\.version=(\S+)$`)
	installBuild  = regexp.MustCompile(`^by "go install \.\.\.@(\S+)"$`)
	checkoutBuild = `from a local checkout with "go build"`
	noGitBuild    = `with "go build" from a tree with no git information`
)

const checkoutRelease = "v1.2.3"

// builtAs builds itos-cc as the Given says, every run then running that
// build.
func (c *cliState) builtAs(how string) error {
	var b Build
	switch {
	case releaseBuild.MatchString(how):
		b = Build{How: "release", Version: releaseBuild.FindStringSubmatch(how)[1]}
	case installBuild.MatchString(how):
		b = Build{How: "install", Version: installBuild.FindStringSubmatch(how)[1]}
	case how == checkoutBuild:
		b = Build{How: "checkout", Version: checkoutRelease}
		c.next = "1.2.4"
	case how == noGitBuild:
		b = Build{How: "no-git"}
	default:
		return fmt.Errorf("no build is made %s", how)
	}
	built, err := c.w.d.Build(c.w.dir, b)
	if err != nil {
		return err
	}
	c.built, c.w.binary = built, built.Binary
	return nil
}

// firstLineIs is whether stdout's first line is text, in which a
// checkout's build reads <next> as the release after the checkout's last,
// <time> as the commit's time, fourteen digits, and <commit> as the first
// twelve characters of its commit.
func (c *cliState) firstLineIs(text string) error {
	first, _, _ := strings.Cut(c.w.stdout, "\n")
	first = strings.TrimSuffix(first, "\r")
	if !strings.Contains(text, "<") {
		if first != text {
			return fmt.Errorf("the first line of stdout is %q, not %q\n%s", first, text, c.w.report())
		}
		return nil
	}
	if c.built.Commit == "" || c.next == "" {
		return fmt.Errorf("%q names a checkout's version, and no Given built one", text)
	}
	pattern := regexp.QuoteMeta(text)
	for placeholder, value := range map[string]string{
		"<next>": regexp.QuoteMeta(c.next), "<time>": `\d{14}`, "<commit>": regexp.QuoteMeta(c.built.Commit[:12]),
	} {
		pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta(placeholder), value)
	}
	if !regexp.MustCompile("^" + pattern + "$").MatchString(first) {
		return fmt.Errorf("the first line of stdout is %q, not %q, the release after %s and commit %s\n%s", first, text, checkoutRelease, c.built.Commit, c.w.report())
	}
	return nil
}

// Flags.

// digits are the runs of digits in a run's output that differ from one run
// to the next: times, and the folders a coverage run makes.
var digits = regexp.MustCompile(`\d+`)

// sameAs is whether the last run, which was no usage error, did what
// itos-cc with args does: the same exit code and stdout, and the same
// stderr but for its numbers.
func (c *cliState) sameAs(args string) error {
	if c.w.last == nil || c.w.exit == 2 {
		return fmt.Errorf("the run to compare was a usage error, or none\n%s", c.w.report())
	}
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	code, stdout, stderr, err := c.w.capture(c.w.last.dir, c.w.last.env, fields...)
	if err != nil {
		return err
	}
	if code != c.w.exit || stdout != c.w.stdout || digits.ReplaceAllString(stderr, "N") != digits.ReplaceAllString(c.w.stderr, "N") {
		return fmt.Errorf("itos-cc %s did otherwise: exit %d\n--- stdout\n%s--- stderr\n%s\n--- the run\n%s", args, code, stdout, stderr, c.w.report())
	}
	return nil
}

// isAPath is whether the run of units listed the units of the files under
// the folder name alone, and no test's.
func (c *cliState) isAPath(name string) error {
	lines := outputLines(filepath.ToSlash(c.w.stdout))
	if c.w.exit != 0 || len(lines) == 0 || lines[0] == "" {
		return fmt.Errorf("the run listed nothing\n%s", c.w.report())
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, name+"/") {
			return fmt.Errorf("%q is no unit under %s\n%s", line, name, c.w.report())
		}
	}
	return nil
}

// The contract with scripts.

// situation sets up the commands of a situation of CLI-20: one for each
// thing the list after its colon names, else the one it names.
func (c *cliState) situation(text string) error {
	items := []string{text}
	if kind, list, ok := strings.Cut(text, ": "); ok && kind != "a temporary failure, such as serve's port being in use" {
		items = splitList(strings.ReplaceAll(list, ", or ", ", "))
	}
	for _, item := range items {
		if !slices.Contains(knownSituations, item) {
			return fmt.Errorf("no command is set up for %q", item)
		}
		c.situations = append(c.situations, situation{what: item})
	}
	return nil
}

// commandEnds runs the command of each situation in a folder of its own,
// each run held to the exit code step.
func (c *cliState) commandEnds() error {
	if len(c.situations) == 0 {
		return errors.New("no situation was set up")
	}
	var each []outcome
	for i, s := range c.situations {
		dir := filepath.Join(c.w.dir, strconv.Itoa(i))
		args, release, err := c.layOut(s.what, dir)
		if err != nil {
			return fmt.Errorf("%s: %w", s.what, err)
		}
		err = c.w.runEnv(dir, c.w.env(), args...)
		if release != nil {
			release()
		}
		if err != nil {
			return err
		}
		each = append(each, outcome{run: *c.w.last, exit: c.w.exit, stdout: c.w.stdout, stderr: c.w.stderr})
	}
	c.w.each = each
	return nil
}

// layOut lays out dir for the command of the situation what and gives its
// words, and what to release once it ran.
func (c *cliState) layOut(what, dir string) ([]string, func(), error) {
	w := c.w
	source := func() error { return w.writeIn(dir, "x.ts", sourceFor("x.ts")) }
	switch what {
	case "it succeeded":
		return []string{"units"}, nil, source()
	case "a mutant survived":
		// Over's > to >= survives: no test gives it 10.
		files := map[string]string{
			"go.mod":        goModule,
			"board.go":      "package board\n\nfunc Over(a int) bool {\n\treturn a > 10\n}\n",
			"board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestOver(t *testing.T) {\n\tif !Over(100) || Over(1) {\n\t\tt.Fatal(\"over\")\n\t}\n}\n",
		}
		for name, text := range files {
			if err := w.writeIn(dir, name, text); err != nil {
				return nil, nil, err
			}
		}
		if err := w.d.Coverage(dir, Coverage{Language: "go"}); err != nil {
			return nil, nil, err
		}
		// Its tests, which mutation run runs over the package, pass with
		// that one mutant, the file's only one, and kill none.
		if err := w.d.Tests(dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./.",
			Surviving: []Site{{File: "board.go", Line: 4, Original: ">", Replacement: ">="}}}); err != nil {
			return nil, nil, err
		}
		return []string{"mutation", "run", "--no-annotate", "--workers", "1", "board.go"}, nil, nil
	case "a function is over a threshold":
		source, lcov := goFile("board.go", []goFunction{{name: "place", cc: 3}})
		if err := w.writeIn(dir, "board.go", source); err != nil {
			return nil, nil, err
		}
		if err := w.writeIn(dir, "lcov.info", lcov); err != nil {
			return nil, nil, err
		}
		return []string{"crap", "--threshold", "5", "--coverage-report", "lcov.info", "board.go"}, nil, nil
	case "a bad flag":
		return []string{"crap", "--no-such-flag"}, nil, source()
	case "an unknown command":
		return []string{"lint"}, nil, source()
	case "path":
		return []string{"crap", "nosuch"}, nil, source()
	case "report":
		return []string{"crap", "--threshold", "30", "--coverage-report", "missing.info"}, nil, source()
	case "a tool":
		// TypeScript with neither Vitest nor Jest, and no c8.
		if err := w.writeIn(dir, "package.json", "{}\n"); err != nil {
			return nil, nil, err
		}
		return []string{"crap", "--threshold", "30"}, nil, source()
	case "a report":
		return []string{"crap", "--threshold", "30", "--use-existing-coverage"}, nil, source()
	case "a git repository":
		for d := dir; ; d = filepath.Dir(d) {
			if w.d.Exists(filepath.Join(d, ".git")) {
				return nil, nil, fmt.Errorf("%s is a repository above the scratch folder: %w", d, godog.ErrSkip)
			}
			if filepath.Dir(d) == d {
				break
			}
		}
		return []string{"crap", "--changed"}, nil, source()
	case "an error itos-cc could not classify":
		// .metrics, where crap writes its snapshot, is a file.
		if err := w.writeIn(dir, ".metrics", "not a folder\n"); err != nil {
			return nil, nil, err
		}
		return []string{"crap", "--no-coverage"}, nil, source()
	case "a temporary failure, such as serve's port being in use":
		port, release, err := w.d.Occupy()
		if err != nil {
			return nil, nil, err
		}
		if err := w.d.Mkdir(dir); err != nil {
			release()
			return nil, nil, err
		}
		return []string{"serve", "--port", strconv.Itoa(port)}, release, nil
	}
	return nil, nil, fmt.Errorf("no command is set up for %q", what)
}

// scoringBesideUnmeasured writes, in the scratch folder, the Go module
// board, whose place makes three decisions no test runs, so it scores 20
// (4² + 4), beside plain, which its test covers, and declares that go test
// measures it; and beside it a TypeScript project with neither Vitest nor
// Jest and no c8, whose coverage no tool measures: a run with a threshold
// below 20 has a problem of each kind.
func (c *cliState) scoringBesideUnmeasured() error {
	fns := []goFunction{{name: "place", cc: 4}, {name: "plain", cc: 1, covered: true}}
	source, lcov := goFile("board.go", fns)
	for name, text := range map[string]string{"go.mod": goModule, "board.go": source, "board_test.go": goTest(fns),
		"package.json": "{}\n", "x.ts": sourceFor("x.ts")} {
		if err := c.w.write(name, text); err != nil {
			return err
		}
	}
	return c.w.d.Coverage(c.w.dir, Coverage{Language: "go", Report: lcov})
}

// ownKeys is whether stdout is one JSON object, schema 1 and ok, holding
// every key the command's help gives its --json object beside schema, ok
// and problems, none of them null.
func (c *cliState) ownKeys() error {
	dec := json.NewDecoder(strings.NewReader(c.w.stdout))
	var object map[string]any
	if err := dec.Decode(&object); err != nil || dec.More() {
		return fmt.Errorf("stdout is not one JSON object: %v\n%s", err, c.w.report())
	}
	if object["schema"] != 1.0 || object["ok"] != true {
		return fmt.Errorf("the object is not schema 1 and ok\n%s", c.w.report())
	}
	command := ""
	for _, a := range c.w.last.args {
		if !strings.HasPrefix(a, "-") {
			command = a
			break
		}
	}
	keys, err := c.jsonKeys(command)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if object[key] == nil {
			return fmt.Errorf("the object lacks %q, a key of %s's --json\n%s", key, command, c.w.report())
		}
	}
	return nil
}

// jsonKeys are the keys of command's --json object its help gives, at the
// object's own level, beside schema, ok and problems.
func (c *cliState) jsonKeys(command string) ([]string, error) {
	_, help, _, err := c.w.capture(c.w.dir, c.w.env(), "help", command)
	if err != nil {
		return nil, err
	}
	_, shape, ok := strings.Cut(help, "Output with --json")
	if !ok {
		return nil, fmt.Errorf("%s's help gives no --json shape", command)
	}
	shape, _, _ = strings.Cut(shape, "\n\n")
	var keys []string
	depth := 0
	for _, m := range regexp.MustCompile(`[{}\[\]]|"[^"]+"`).FindAllString(shape, -1) {
		switch m {
		case "{", "[":
			depth++
		case "}", "]":
			depth--
		default:
			if key := strings.Trim(m, `"`); depth == 1 && !slices.Contains([]string{"schema", "ok", "problems"}, key) {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s's --json shape gives no key of its own", command)
	}
	return keys, nil
}

// progressOnStderr is whether stderr shows the coverage command itos-cc
// ran and go test's output, and stdout neither.
func (c *cliState) progressOnStderr() error {
	for _, want := range []struct {
		what, pattern string
	}{{"the coverage command", `(?m)^itos-cc: coverage `}, {"go test's output", `(?m)^ok\s`}} {
		re := regexp.MustCompile(want.pattern)
		if !re.MatchString(c.w.stderr) || re.MatchString(c.w.stdout) {
			return fmt.Errorf("%s is not on stderr alone\n%s", want.what, c.w.report())
		}
	}
	return nil
}

// stdoutIs is whether stdout is the text exactly, with its last line's
// newline.
func (c *cliState) stdoutIs(text *godog.DocString) error {
	if got := strings.ReplaceAll(c.w.stdout, "\r\n", "\n"); got != text.Content+"\n" {
		return fmt.Errorf("stdout is\n%s\nnot\n%s\n%s", got, text.Content, c.w.report())
	}
	return nil
}

// The mutation group.

// stdoutNames is whether stdout lists each subcommand on a line of its
// own.
func (c *cliState) stdoutNames(first, second string) error {
	for _, name := range []string{first, second} {
		if !regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(name) + `\s`).MatchString(c.w.stdout) {
			return fmt.Errorf("stdout lists no subcommand %s\n%s", name, c.w.report())
		}
	}
	return nil
}

// stderrNames is whether a line of stderr naming the subcommands names
// each.
func (c *cliState) stderrNames(first, second string) error {
	for _, line := range outputLines(c.w.stderr) {
		if !strings.Contains(line, "subcommands") {
			continue
		}
		if regexp.MustCompile(`\b`+regexp.QuoteMeta(first)+`\b`).MatchString(line) &&
			regexp.MustCompile(`\b`+regexp.QuoteMeta(second)+`\b`).MatchString(line) {
			return nil
		}
	}
	return fmt.Errorf("stderr names no subcommands %s and %s\n%s", first, second, c.w.report())
}

// eachSurvivor is whether the run, made with --json, reports a problem of
// rule for each mutant its plain line says survived, one at least.
func (c *cliState) eachSurvivor(rule string) error {
	m := regexp.MustCompile(`(\d+) survived`).FindStringSubmatch(c.w.stdout)
	if m == nil {
		return fmt.Errorf("stdout says no count of survivors\n%s", c.w.report())
	}
	survived, _ := strconv.Atoi(m[1])
	object, report, err := c.w.lastObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	count := 0
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == rule {
			count++
		}
	}
	if survived == 0 || count != survived {
		return fmt.Errorf("%d %s problems for %d survivors\n%s", count, rule, survived, report)
	}
	return nil
}

// A line of mutation list: the file, line and column, the original and
// the replacement, and the function.
var siteLine = regexp.MustCompile("^(\\S+):(\\d+):(\\d+) `([^`]+)` → `([^`]*)` in (\\S+#\\S+)$")

// listsSites is whether stdout lists exactly file's mutation sites, each a
// line giving its line, column, function, original and replacement.
func (c *cliState) listsSites(file string) error {
	var got []string
	for _, line := range outputLines(filepath.ToSlash(c.w.stdout)) {
		m := siteLine.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("%q is no site's line\n%s", line, c.w.report())
		}
		got = append(got, strings.Join([]string{m[1], m[2], m[3], m[6], m[4], m[5]}, " "))
	}
	var want []string
	for _, site := range xSites {
		if strings.HasPrefix(site, file+" ") {
			want = append(want, site)
		}
	}
	if len(want) == 0 || !slices.Equal(got, want) {
		return fmt.Errorf("the sites listed are %v, not %v\n%s", got, want, c.w.report())
	}
	return c.w.exitsWith(0)
}

// noTestRuns is whether the project's test script left nothing behind,
// and nothing was written under .metrics or said of a baseline.
func (c *cliState) noTestRuns() error {
	for _, name := range []string{"test-ran", ".metrics"} {
		if c.w.d.Exists(filepath.Join(c.w.dir, name)) {
			return fmt.Errorf("%s is there: a test or coverage command ran\n%s", name, c.w.report())
		}
	}
	if strings.Contains(c.w.stderr, "baseline") {
		return fmt.Errorf("stderr tells of a baseline\n%s", c.w.report())
	}
	return nil
}
