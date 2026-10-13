// The steps. itos-cc is a black box here: each scenario gets a fresh
// scratch folder, runs itos-cc in it, and reads its exit code and its
// output. The steps reach that world only through a Driver: the binary one
// (binary.go) runs the itos-cc TestFeatures built from this tree as another
// process on the disk, and cmd/itos-cc's tests run the same steps in
// process (decision 44). Nothing here imports or reads itos-cc's code, so
// the steps judge it by its command line alone. Harvested from itos's
// (github.com/donvargax/itos, features/steps_test.go) and the owner's
// itos-template's, its domain's steps left behind.
package features

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// Driver is the world the steps reach: folders and files, git, and runs of
// itos-cc. Every path it takes is absolute, in the system's form.
type Driver interface {
	// Folder makes a fresh, empty folder and gives its absolute path.
	Folder() (string, error)
	// Write writes data to the file at path, making the folders above it.
	Write(path string, data []byte) error
	// Read is the content of the file at path.
	Read(path string) ([]byte, error)
	// Mkdir makes the folder at path, whose parent is there.
	Mkdir(path string) error
	// Exists is whether a file or folder is at path.
	Exists(path string) bool
	// Remove removes the file or folder at path and everything below it.
	Remove(path string) error
	// Git runs git with args in the folder dir, for a fixture that needs a
	// repository: init, add and commit.
	Git(dir string, args ...string) error
	// Run runs itos-cc as r says and gives what it did. Its error is a run
	// that could not be made, never an exit code other than 0.
	Run(r Run) (Result, error)
}

// Run is one run of itos-cc: the folder it runs in, the variables the
// scenario sets in its environment as NAME=value, the text of its standard
// input, "" for none, and its arguments.
type Run struct {
	Dir   string
	Env   []string
	Stdin string
	Args  []string
}

// Result is what a run of itos-cc did: its exit code and its output.
type Result struct {
	Exit           int
	Stdout, Stderr string
}

// stampedVersion is the version the binary under test is stamped with, as a
// release's build stamps the release's, so a scenario can tell the stamp
// from the version a build without one says.
const stampedVersion = "1.2.3-features"

// stampedCommit is the commit the binary under test is stamped with, as a
// release's build stamps the commit it was built from, so a scenario can
// name the commit the binary says it was built from.
const stampedCommit = "0123456789abcdef0123456789abcdef01234567"

// Converted are the feature files the steps run, those whose scenarios
// have steps: count.feature, the template's sample, every scenario of it
// @wip (T-26), units.feature, the first command moved onto kong (T-32), and
// dry.feature and scrap.feature, each converted before its command moves so
// the move is held by its scenarios (T-37, T-41). A command that moves
// converts its own file and adds it here, its scenarios' text unchanged
// (q-52).
var Converted = []string{"count.feature", "units.feature", "dry.feature", "scrap.feature"}

// One scenario's state.
type world struct {
	d   Driver // the world the steps reach
	dir string // the scratch folder

	exit           int
	stdout, stderr string

	stdin string // the text the run reads from its standard input, "" for none

	// last is the last run of itos-cc, which the problem steps run again
	// with --json when it was run without, nil before one.
	last *run
	// problem is the problem of the last run the problem step found, its
	// fix what the fix step reads, nil before one.
	problem map[string]any
}

// run is one run of itos-cc: where, with the variables the scenario set
// and with which words.
type run struct {
	dir  string
	env  []string
	args []string
}

// Initialize gives sc every step of the converted feature files, each
// reaching the world through d.
func Initialize(sc *godog.ScenarioContext, d Driver) {
	initialize(sc, d)
}

// initialize is Initialize, and gives each of own sc and the scenario's
// world, for the steps only one driver can take, before the scratch folder
// is removed after the scenario: the binary driver's interrupt
// (interrupt_test.go).
func initialize(sc *godog.ScenarioContext, d Driver, own ...func(*godog.ScenarioContext, *world)) {
	w := &world{d: d}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.setUp()
	})
	for _, steps := range own {
		steps(sc, w)
	}
	// The scenario's error is godog's already: returned here, it would be
	// reported twice, as the hook's and as the step's.
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		_ = w.d.Remove(w.dir)
		return ctx, nil
	})

	// Split before anything else is expanded, so a word with a space in it
	// stays one argument. Quotes also preserve empty words for completion
	// requests.
	sc.Step(`^itos-cc runs with "([^"]*)"$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^itos-cc runs with "([^"]*)" and the environment "([^"]*)"$`, func(args, vars string) error {
		return w.runWith(w.withEnv(vars), args)
	})

	sc.Step(`^it exits with code (\d+)$`, w.exitsWith)
	sc.Step(`^the exit code is (\d+)$`, w.exitsWith)
	// The problem a run reports, by its rule and the flag it names, and the
	// flag its fix suggests.
	sc.Step(`^the problem is "([^"]*)", with flag "([^"]*)"$`, w.problemIs)
	sc.Step(`^its fix suggests "([^"]*)"$`, w.fixSuggests)
	sc.Step(`^its standard output lists "([^"]*)"$`, func(text string) error {
		if !slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output does not list %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not list "([^"]*)"$`, func(text string) error {
		if slices.Contains(outputLines(w.stdout), text) {
			return fmt.Errorf("standard output lists %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the last line of its standard output is "([^"]*)"$`, func(text string) error {
		lines := outputLines(w.stdout)
		last := strings.TrimSuffix(lines[len(lines)-1], "\r")
		if last != text {
			return fmt.Errorf("the last line of standard output is %q, not %q\n%s", last, text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stdout, text) {
			return fmt.Errorf("standard output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its standard output is empty$`, func() error {
		if w.stdout != "" {
			return fmt.Errorf("standard output is not empty\n%s", w.report())
		}
		return nil
	})
	sc.Step(`^its error output says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^its error output does not say "([^"]*)"$`, func(text string) error {
		if strings.Contains(w.stderr, text) {
			return fmt.Errorf("error output says %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^the first line of its standard output is "([^"]*)" and the stamped version$`, func(name string) error {
		return w.firstLineIs(name + " " + stampedVersion)
	})
	sc.Step(`^the second line of its standard output is "([^"]*)" and the stamped commit$`, func(word string) error {
		return w.secondLineIs(word + " " + stampedCommit)
	})

	// The --json object, as a script reads it.
	sc.Step(`^its JSON output names the problem "([^"]*)"$`, w.jsonNamesProblem)
	sc.Step(`^its JSON output gives file "([^"]*)", (\d+) lines and (\d+) words$`, w.jsonGivesCounts)
	sc.Step(`^its JSON output gives each problem a rule, a message and a fix$`, w.jsonProblemsComplete)
	sc.Step(`^its JSON output gives the stamped version and commit$`, w.jsonGivesBuild)
	sc.Step(`^its standard output is one line$`, w.stdoutIsOneLine)

	// The steps of the feature files written in the short form, a run of
	// "itos-cc ..." and what stdout and stderr show, which units.feature
	// and dry.feature share.
	sc.Step(`^I run "itos-cc ?([^"]*)"$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^I run "itos-cc ?([^"]*)" without options$`, func(args string) error {
		return w.runWith(w.env(), args)
	})
	sc.Step(`^stdout is one object with "schema": 1, "ok", (.+)$`, w.isOneObjectWith)
	sc.Step(`^stdout shows:$`, w.stdoutShows)
	sc.Step(`^stderr says "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stderr, text) {
			return fmt.Errorf("stderr does not say %q\n%s", text, w.report())
		}
		return nil
	})
	sc.Step(`^"([^"]*)" is an empty list$`, w.isEmptyList)
	sc.Step(`^"([^"]*)" and "([^"]*)" are empty lists, not null$`, func(first, second string) error {
		if err := w.isEmptyList(first); err != nil {
			return err
		}
		return w.isEmptyList(second)
	})

	// The fixtures a scenario asks for. godog takes one parameter per
	// capturing group and none variadic, so the second text is its own
	// group, empty where the scenario names one.
	sc.Step(`^the file "([^"]*)" contains "([^"]*)"(?:, "([^"]*)")?$`, func(name, first, second string) error {
		texts := []string{first}
		if second != "" {
			texts = append(texts, second)
		}
		return w.fileContains(name, texts...)
	})
	sc.Step(`^the file "([^"]*)" holds a line of (\d+) characters$`, w.fileHoldsLine)
	sc.Step(`^standard input holds "([^"]*)"$`, func(text string) error {
		w.stdin = text
		return nil
	})
	sc.Step(`^an empty folder "([^"]*)"$`, w.emptyFolder)
	// itos-cc.yaml, the project's settings at the root of the scratch
	// folder, leaving out of what is measured the sources a glob names.
	sc.Step(`^itos-cc\.yaml excludes "([^"]*)"$`, func(glob string) error {
		return w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n")
	})

	// The steps of each converted feature file, after the world's, so a
	// file's own Before runs in the scratch folder made above.
	unitSteps(sc, w)
	drySteps(sc, w)
	scrapSteps(sc, w)
}

// setUp makes the scenario's scratch folder.
func (w *world) setUp() error {
	w.last, w.problem = nil, nil
	var err error
	w.dir, err = w.d.Folder()
	return err
}

// env is the variables a scenario sets in every run's environment: none.
// The driver gives each run the rest.
func (w *world) env() []string { return nil }

// withEnv is the scenarios' environment with each NAME=value of vars, a
// semicolon-separated list, set as given.
func (w *world) withEnv(vars string) []string {
	env := w.env()
	for _, one := range strings.Split(vars, ";") {
		if name, value, ok := strings.Cut(one, "="); ok {
			env = setEnv(env, name, value)
		}
	}
	return env
}

// When steps.

// runWith runs itos-cc in the scratch folder with args, split as a
// scenario wrote them, in the environment env.
func (w *world) runWith(env []string, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	return w.runEnv(w.dir, env, fields...)
}

// commandWords splits a scenario's command line without involving a shell.
// Quotes group words and preserve an explicitly empty argument, as the
// completion protocol needs for a new token. Backslashes remain literal, so
// Windows paths survive the harness.
func commandWords(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	for _, r := range line {
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			word.WriteRune(r)
			started = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in scenario command %q", line)
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

// setEnv is env with the variable name, in any case, set to value.
func setEnv(env []string, name, value string) []string {
	var out []string
	for _, kv := range env {
		if n, _, _ := strings.Cut(kv, "="); !strings.EqualFold(n, name) {
			out = append(out, kv)
		}
	}
	return append(out, name+"="+value)
}

// runEnv runs itos-cc with args in the folder dir with the variables env
// sets, its exit code and output what the Then steps read.
func (w *world) runEnv(dir string, env []string, args ...string) error {
	w.last = &run{dir: dir, env: env, args: args}
	var err error
	w.exit, w.stdout, w.stderr, err = w.capture(dir, env, args...)
	return err
}

// capture runs itos-cc with args in the folder dir with the variables env
// sets, reading the scenario's standard input, and gives its exit code and
// output.
func (w *world) capture(dir string, env []string, args ...string) (code int, stdout, stderr string, err error) {
	result, err := w.d.Run(Run{Dir: dir, Env: env, Stdin: w.stdin, Args: args})
	return result.Exit, result.Stdout, result.Stderr, err
}

// Fixture steps.

// fileContains writes name in the scratch folder, one line per text the
// scenario named, each ending in the newline a text file's last line has.
func (w *world) fileContains(name string, texts ...string) error {
	if len(texts) == 0 || texts[0] == "" {
		return fmt.Errorf("the file %q is given no text to contain", name)
	}
	body := strings.Join(texts, "\n") + "\n"
	return w.d.Write(filepath.Join(w.dir, name), []byte(body))
}

// fileHoldsLine writes name in the scratch folder holding one line of count
// characters, long enough to beat a token limit a reader might have, and no
// other line.
func (w *world) fileHoldsLine(name string, count int) error {
	body := strings.Repeat("x", count) + "\n"
	return w.d.Write(filepath.Join(w.dir, name), []byte(body))
}

// emptyFolder makes a folder in the scratch directory.
func (w *world) emptyFolder(name string) error {
	return w.d.Mkdir(filepath.Join(w.dir, name))
}

// write writes text to name, a slash-separated path in the scratch folder,
// making the folders above it.
func (w *world) write(name, text string) error {
	return w.writeIn(w.dir, name, text)
}

// writeIn writes text to name, a slash-separated path in dir, making the
// folders above it.
func (w *world) writeIn(dir, name, text string) error {
	return w.d.Write(filepath.Join(dir, filepath.FromSlash(name)), []byte(text))
}

// readIn is the text of name, a slash-separated path in dir.
func (w *world) readIn(dir, name string) ([]byte, error) {
	return w.d.Read(filepath.Join(dir, filepath.FromSlash(name)))
}

// git runs git with args in the scratch folder, for a fixture that needs a
// repository.
func (w *world) git(args ...string) error {
	return w.d.Git(w.dir, args...)
}

// Then steps.

func (w *world) report() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", w.exit, w.stdout, w.stderr)
}

func outputLines(output string) []string {
	return strings.Split(strings.TrimSuffix(output, "\n"), "\n")
}

func (w *world) exitsWith(code int) error {
	if w.exit != code {
		return fmt.Errorf("itos-cc exited %d, not %d\n%s", w.exit, code, w.report())
	}
	return nil
}

// secondLineIs is whether standard output's second line is text, a missing
// line read as an empty one and a line ending in \r\n as one in \n.
func (w *world) secondLineIs(text string) error {
	_, rest, _ := strings.Cut(w.stdout, "\n")
	second, _, _ := strings.Cut(rest, "\n")
	if second = strings.TrimSuffix(second, "\r"); second != text {
		return fmt.Errorf("the second line of standard output is %q, not %q\n%s", second, text, w.report())
	}
	return nil
}

// firstLineIs is whether standard output's first line is text, a line ending
// in \r\n read as one in \n.
func (w *world) firstLineIs(text string) error {
	first, _, _ := strings.Cut(w.stdout, "\n")
	if first = strings.TrimSuffix(first, "\r"); first != text {
		return fmt.Errorf("the first line of standard output is %q, not %q\n%s", first, text, w.report())
	}
	return nil
}

// The JSON steps read standard output as the one object --json prints.

// json is the object standard output holds, one line of JSON as --json
// prints it.
func (w *world) json() (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return nil, fmt.Errorf("standard output is not one JSON object: %v\n%s", err, w.report())
	}
	return object, nil
}

// jsonNamesProblem is whether the object's problems list one with that rule.
func (w *world) jsonNamesProblem(rule string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		if problem["rule"] == rule {
			return nil
		}
	}
	return fmt.Errorf("no problem of the JSON output has the rule %q\n%s", rule, w.report())
}

// problemIs is whether the last run reports a problem of that rule naming
// that flag. The plain output names no rule, so the run is made again with
// --json before its words, as a script would make it, unless it had --json
// already: that run must exit as the plain one did, and the plain one's
// error output must print the problem's message.
func (w *world) problemIs(rule, flag string) error {
	if w.last == nil {
		return errors.New("no run of itos-cc to read a problem of")
	}
	stdout, report := w.stdout, w.report()
	if !slices.Contains(w.last.args, "--json") {
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, append([]string{"--json"}, w.last.args...)...)
		if err != nil {
			return err
		}
		report = fmt.Sprintf("%s\n--- with --json, exit %d\n--- stdout\n%s--- stderr\n%s", report, code, out, errOut)
		if code != w.exit {
			return fmt.Errorf("with --json itos-cc exited %d, not %d as without it\n%s", code, w.exit, report)
		}
		stdout = out
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return fmt.Errorf("the output with --json is not one JSON object: %v\n%s", err, report)
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		if problem["rule"] != rule || problem["flag"] != flag {
			continue
		}
		if message, _ := problem["message"].(string); message == "" || !strings.Contains(w.stderr, message) {
			return fmt.Errorf("the error output does not print the problem's message %q\n%s", message, report)
		}
		w.problem = problem
		return nil
	}
	return fmt.Errorf("no problem has the rule %q and the flag %q\n%s", rule, flag, report)
}

// fixSuggests is whether the fix of the problem problemIs found asks
// whether the person meant flag, in the words the command line uses, and
// the error output prints that fix.
func (w *world) fixSuggests(flag string) error {
	if w.problem == nil {
		return errors.New("no problem found to read the fix of")
	}
	fix, _ := w.problem["fix"].(string)
	if want := "Did you mean " + flag + "?"; !strings.Contains(fix, want) {
		return fmt.Errorf("the problem's fix %q does not say %q\n%s", fix, want, w.report())
	}
	if !strings.Contains(w.stderr, fix) {
		return fmt.Errorf("the error output does not print the problem's fix %q\n%s", fix, w.report())
	}
	return nil
}

// jsonGivesCounts is whether the success object's file is file, its lines
// the count and its words the count.
func (w *world) jsonGivesCounts(file string, lines, words int) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if object["ok"] != true {
		return fmt.Errorf("the JSON output's ok is %v, not true\n%s", object["ok"], w.report())
	}
	for _, want := range []struct {
		key  string
		got  any
		want any
	}{
		{"file", object["file"], file},
		{"lines", numberOf(object["lines"]), lines},
		{"words", numberOf(object["words"]), words},
	} {
		if want.got != want.want {
			return fmt.Errorf("the JSON output's %s is %v, not %v\n%s", want.key, want.got, want.want, w.report())
		}
	}
	return nil
}

// jsonGivesBuild is whether the success object of version --json is schema
// 1 and ok, and gives the version and the commit the harness stamped.
func (w *world) jsonGivesBuild() error {
	object, err := w.json()
	if err != nil {
		return err
	}
	for _, want := range []struct {
		key  string
		want any
	}{
		{"schema", 1.0},
		{"ok", true},
		{"version", stampedVersion},
		{"commit", stampedCommit},
	} {
		if got := object[want.key]; got != want.want {
			return fmt.Errorf("the JSON output's %s is %v, not %v\n%s", want.key, got, want.want, w.report())
		}
	}
	return nil
}

// stdoutIsOneLine is whether standard output is exactly one line, ended by
// its newline, as --json prints its object.
func (w *world) stdoutIsOneLine() error {
	if strings.Count(w.stdout, "\n") != 1 || !strings.HasSuffix(w.stdout, "\n") {
		return fmt.Errorf("standard output is not one line\n%s", w.report())
	}
	return nil
}

// objectKeys is a list of quoted keys as a sentence writes it: "a", "b",
// and "c", or and "a" alone.
var objectKeys = regexp.MustCompile(`^(?:and )?"[^"]+"(?:(?:, and |, | and )"[^"]+")*$`)

// isOneObjectWith is whether standard output is one JSON object, schema 1,
// with ok and every key of the list ("a", "b", and "c"), none of them null.
func (w *world) isOneObjectWith(list string) error {
	if !objectKeys.MatchString(list) {
		return fmt.Errorf("%s is no list of quoted keys", list)
	}
	dec := json.NewDecoder(strings.NewReader(w.stdout))
	var object map[string]any
	if err := dec.Decode(&object); err != nil {
		return fmt.Errorf("standard output is not a JSON object: %v\n%s", err, w.report())
	}
	if dec.More() {
		return fmt.Errorf("standard output holds more than one JSON value\n%s", w.report())
	}
	if _, ok := object["ok"].(bool); !ok || object["schema"] != 1.0 {
		return fmt.Errorf("the object lacks \"schema\": 1 or \"ok\"\n%s", w.report())
	}
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list, -1) {
		if object[m[1]] == nil {
			return fmt.Errorf("the object lacks %q\n%s", m[1], w.report())
		}
	}
	return nil
}

// stdoutShows is whether standard output is the text, line for line, with
// its last line's newline. A path printed with the system's separator reads
// as one with slashes, as the scenarios write paths.
func (w *world) stdoutShows(text *godog.DocString) error {
	got := strings.ReplaceAll(filepath.ToSlash(w.stdout), "\r\n", "\n")
	if want := text.Content + "\n"; got != want {
		return fmt.Errorf("stdout is\n%s\nnot\n%s\n%s", got, want, w.report())
	}
	return nil
}

// isEmptyList is whether the object standard output holds has key, a list
// with nothing in it, not null.
func (w *world) isEmptyList(key string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if list, ok := object[key].([]any); !ok || len(list) != 0 {
		return fmt.Errorf("%q is %v, not an empty list\n%s", key, object[key], w.report())
	}
	return nil
}

// jsonProblemsComplete is whether the object's problems all carry a rule, a
// message and a fix, and there is one.
func (w *world) jsonProblemsComplete() error {
	object, err := w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) == 0 {
		return fmt.Errorf("the JSON output lists no problem\n%s", w.report())
	}
	for i, one := range problems {
		problem, _ := one.(map[string]any)
		for _, key := range []string{"rule", "message", "fix"} {
			if text, _ := problem[key].(string); text == "" {
				return fmt.Errorf("problem %d of the JSON output has no %s\n%s", i+1, key, w.report())
			}
		}
	}
	return nil
}

// numberOf is the number a decoded JSON value holds, a float64 as encoding/
// json decodes every number into.
func numberOf(value any) int {
	number, _ := value.(float64)
	return int(number)
}

// TagFilter is godog's tag filter for a selection, the feature files in dir:
// every live scenario of the Converted files without one, else the live
// scenarios of the Converted files carrying a tag the expression matches,
// with the tags it matches only in other files, which the steps do not
// run. An expression that matches no tag of a converted file is an error,
// so a selection never runs nothing.
//
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so a selection is that expression,
// turned here into godog's filter. A scenario tagged @wip never runs.
func TagFilter(selection, dir string) (filter string, unconverted []string, err error) {
	if selection == "" {
		return "~@wip", nil, nil
	}
	pattern, err := regexp.Compile(selection)
	if err != nil {
		return "", nil, err
	}
	var files []string
	for _, name := range Converted {
		files = append(files, filepath.Join(dir, name))
	}
	tags, err := featureTags(files)
	if err != nil {
		return "", nil, err
	}
	var each []string
	for _, tag := range tags {
		if pattern.MatchString(tag) {
			each = append(each, tag+"&&~@wip")
		}
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return "", nil, err
	}
	everyTag, err := featureTags(all)
	if err != nil {
		return "", nil, err
	}
	for _, tag := range everyTag {
		if pattern.MatchString(tag) && !slices.Contains(tags, tag) {
			unconverted = append(unconverted, tag)
		}
	}
	if len(each) == 0 {
		return "", unconverted, &noMatch{selection, unconverted}
	}
	return strings.Join(each, ","), unconverted, nil
}

type noMatch struct {
	selection   string
	unconverted []string
}

func (e *noMatch) Error() string {
	if len(e.unconverted) > 0 {
		return "no scenario this harness runs has a tag that " + e.selection + " matches: " +
			strings.Join(e.unconverted, " ") + " are in feature files with no steps yet (q-52), held by cmd/itos-cc's Go tests"
	}
	return "no scenario has a tag that " + e.selection + " matches"
}

// Every tag written in files, once each, sorted.
func featureTags(files []string) ([]string, error) {
	seen := map[string]bool{}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		lines := bufio.NewScanner(f)
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if !strings.HasPrefix(line, "@") {
				continue
			}
			for _, word := range strings.Fields(line) {
				if strings.HasPrefix(word, "#") {
					break
				}
				if strings.HasPrefix(word, "@") {
					seen[word] = true
				}
			}
		}
		_ = f.Close()
		if err := lines.Err(); err != nil {
			return nil, err
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}
