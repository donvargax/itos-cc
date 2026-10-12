// The steps. itos-cc is a black box here: each scenario gets a
// scratch folder in a temporary directory, runs the binary TestFeatures
// built from this tree in it, and reads its exit code and its output.
// Nothing here imports or reads itos-cc's code, so the steps judge
// it by its command line alone. Harvested from itos's (github.com/donvargax/
// itos, features/steps_test.go) and the owner's itos-template's, its domain's
// steps left behind.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/cucumber/godog"
)

// One scenario's state.
type world struct {
	root string // the itos-cc checkout: where go.mod is
	bin  string // the itos-cc binary under test
	dir  string // the scratch folder

	exit           int
	stdout, stderr string

	stdin string // the text the run reads from its standard input, "" for none

	running *running // the run started with its standard input left open, nil for none

	// last is the last run of itos-cc, which the problem steps run again
	// with --json when it was run without, nil before one.
	last *run
	// problem is the problem of the last run the problem step found, its
	// fix what the fix step reads, nil before one.
	problem map[string]any
}

// run is one run of itos-cc: where, in what environment and with which
// words.
type run struct {
	dir  string
	env  []string
	args []string
}

func initializeScenario(sc *godog.ScenarioContext, root, bin string) {
	w := &world{root: root, bin: bin}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.setUp()
	})
	// The scenario's error is godog's already: returned here, it would be
	// reported twice, as the hook's and as the step's.
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.running.stop()
		_ = os.RemoveAll(w.dir)
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
	// A run that waits on its input until something ends it, and the
	// interrupt that does (interrupt_test.go).
	sc.Step(`^itos-cc runs with "([^"]*)" and its standard input left open$`, w.startWithInputOpen)
	sc.Step(`^it is interrupted while it waits$`, w.interruptWhileWaiting)

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
	if w.dir, err = os.MkdirTemp("", "itos-cc-features-"); err != nil {
		return err
	}
	return nil
}

// The folder holding go.mod, above the working directory go test gives.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}

// The environment every command runs in: the caller's, less what would make
// git, itos or itos-cc read anything but the scratch folder (a
// hook's GIT_DIR, the caller's ITOS_CC_ settings, CI's), with no
// global or system git config and a fixed identity, and the caller's PATH
// without its claude, its itos, its itos extensions or a git that is an itos
// (callerPath).
func (w *world) env() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "GIT_") || strings.HasPrefix(name, "ITOS_") ||
			strings.HasPrefix(name, "ITOS_CC_") || strings.HasPrefix(name, "GITHUB_") ||
			strings.HasPrefix(name, "GH_") || name == "CI" || strings.EqualFold(name, "PATH") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=itos-cc features",
		"GIT_AUTHOR_EMAIL=features@localhost",
		"GIT_COMMITTER_NAME=itos-cc features",
		"GIT_COMMITTER_EMAIL=features@localhost",
		"PATH="+callerPath(),
	)
}

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

// The programs no scenario reaches on the caller's PATH: its claude, its itos
// and its itos extensions (itos-*). An itos installed on the machine would
// run its own policy where a scenario meant only itos-cc (itos's
// T-087), and itos runs and lists every extension it finds, so one installed
// on the machine would change what a scenario sees (itos's T-080).
var hiddenAlways = []string{"claude", "itos", "itos-*"}

var (
	hiddenPathsMu sync.Mutex
	hiddenPaths   = map[string]string{}
	callerPathDir string
)

// callerPath is the caller's PATH with none of hiddenAlways on it, so no
// scenario reaches the Claude Code or the itos of the machine it runs on:
// each folder holding one is replaced by a folder of links to everything else
// in it, or left out where links cannot be made (windows without the right to
// make them).
func callerPath() string { return pathHiding(hiddenAlways...) }

// pathHiding is the caller's PATH with none of the programs named on it, nor
// a git that is an itos, made once a run for each set of names. A git shim of
// the caller's (itos git-shim install) would run the global itos for a
// scenario's git command; it is told from the real git by the rule itos
// itself goes by (isItos, git_test.go), so every git a scenario's commands
// start is the real one, as the steps' own are (gitBin, itos's T-104).
func pathHiding(names ...string) string {
	hiddenPathsMu.Lock()
	defer hiddenPathsMu.Unlock()
	key := strings.Join(names, " ")
	if text, ok := hiddenPaths[key]; ok {
		return text
	}
	var folders []string
	for i, folder := range filepath.SplitList(os.Getenv("PATH")) {
		hidden := func(e os.DirEntry) bool {
			return isOneOf(e, names) || isOneOf(e, []string{"git"}) && isItos(filepath.Join(folder, e.Name()))
		}
		entries, err := os.ReadDir(folder)
		if err != nil || !slices.ContainsFunc(entries, hidden) {
			folders = append(folders, folder)
			continue
		}
		if callerPathDir == "" {
			if callerPathDir, err = os.MkdirTemp("", "itos-cc-features-path-"); err != nil {
				continue
			}
		}
		links := filepath.Join(callerPathDir, strconv.Itoa(len(hiddenPaths)), strconv.Itoa(i))
		if linkAllBut(folder, links, entries, hidden) == nil {
			folders = append(folders, links)
		}
	}
	text := strings.Join(folders, string(os.PathListSeparator))
	hiddenPaths[key] = text
	return text
}

// removeCallerPath removes the folders callerPath and pathHiding made.
func removeCallerPath() {
	if callerPathDir != "" {
		_ = os.RemoveAll(callerPathDir)
	}
}

// isOneOf is whether a folder's entry is a program of the names the PATH
// would find: the name, or on windows the name with any extension, in any
// case. A name ending in * is a prefix, so itos-* is every itos extension.
func isOneOf(e os.DirEntry, names []string) bool {
	name := e.Name()
	same := func(a, b string) bool { return a == b }
	starts := strings.HasPrefix
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
		same = strings.EqualFold
		starts = func(s, prefix string) bool {
			return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
		}
	}
	return slices.ContainsFunc(names, func(n string) bool {
		if prefix, ok := strings.CutSuffix(n, "*"); ok {
			return starts(name, prefix)
		}
		return same(name, n)
	})
}

// linkAllBut makes links a folder of links to every entry of folder but the
// hidden ones.
func linkAllBut(folder, links string, entries []os.DirEntry, hidden func(os.DirEntry) bool) error {
	if err := os.MkdirAll(links, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if hidden(e) {
			continue
		}
		if err := os.Symlink(filepath.Join(folder, e.Name()), filepath.Join(links, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// When steps.

// runWith runs itos-cc in the scratch folder with args, split as a
// scenario wrote them, in the environment env.
func (w *world) runWith(env []string, args string) error {
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	return w.runEnv(w.dir, env, w.bin, fields...)
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

// runEnv runs program in the folder dir in the environment env, its exit
// code and output what the Then steps read.
func (w *world) runEnv(dir string, env []string, program string, args ...string) error {
	if program == w.bin {
		w.last = &run{dir: dir, env: env, args: args}
	}
	var err error
	w.exit, w.stdout, w.stderr, err = w.capture(dir, env, program, args...)
	return err
}

// capture runs program in the folder dir in the environment env, reading
// the scenario's standard input, and gives its exit code and output.
func (w *world) capture(dir string, env []string, program string, args ...string) (code int, stdout, stderr string, err error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = dir
	cmd.Env = env
	if w.stdin != "" {
		cmd.Stdin = strings.NewReader(w.stdin)
	}
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, out.String(), errOut.String(), nil
	case errors.As(err, &exit):
		return exit.ExitCode(), out.String(), errOut.String(), nil
	default:
		return 0, out.String(), errOut.String(), fmt.Errorf("running %s: %w", program, err)
	}
}

// Fixture steps.

// fileContains writes name in the scratch folder, one line per text the
// scenario named, each ending in the newline a text file's last line has.
func (w *world) fileContains(name string, texts ...string) error {
	if len(texts) == 0 || texts[0] == "" {
		return fmt.Errorf("the file %q is given no text to contain", name)
	}
	body := strings.Join(texts, "\n") + "\n"
	return os.WriteFile(filepath.Join(w.dir, name), []byte(body), 0o600)
}

// fileHoldsLine writes name in the scratch folder holding one line of count
// characters, long enough to beat a token limit a reader might have, and no
// other line.
func (w *world) fileHoldsLine(name string, count int) error {
	body := strings.Repeat("x", count) + "\n"
	return os.WriteFile(filepath.Join(w.dir, name), []byte(body), 0o600)
}

// emptyFolder makes a folder in the scratch directory.
func (w *world) emptyFolder(name string) error {
	return os.Mkdir(filepath.Join(w.dir, name), 0o755)
}

// write writes text to name, a slash-separated path in the scratch folder,
// making the folders above it.
func (w *world) write(name, text string) error {
	return writeIn(w.dir, name, text)
}

// writeIn writes text to name, a slash-separated path in dir, making the
// folders above it.
func writeIn(dir, name, text string) error {
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

// git runs the real git in the scratch folder, in the scenarios'
// environment, for a fixture that needs a repository.
func (w *world) git(args ...string) error {
	bin, err := realGit()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = w.dir
	cmd.Env = w.env()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// realGit is the first git on the scenarios' PATH (callerPath) that is no
// itos, the one every command a scenario starts finds.
func realGit() (string, error) {
	for _, folder := range filepath.SplitList(callerPath()) {
		for _, name := range []string{"git.exe", "git"} {
			path := filepath.Join(folder, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() && !isItos(path) {
				return path, nil
			}
		}
	}
	return "", errors.New("no git on the PATH")
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
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, w.bin, append([]string{"--json"}, w.last.args...)...)
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
