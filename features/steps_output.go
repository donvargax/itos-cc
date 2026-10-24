package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps every feature shares that check what a run printed and its exit
// code.

func (w *world) report() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", w.exit, w.stdout, w.stderr)
}

// stderrSays is whether stderr holds text, \" in it a quote, in each run
// the When made. In a text naming <dir>, that stands for the folder of the
// project the Given made, as itos-cc names it from the scratch folder, and
// "..." or "…" for the rest of what it says there. A text that goes on
// past its said part with `" and names <a>, <b> and <c>` is a line that
// begins with the said part and names each, quoted or not (saysNaming).
func (w *world) stderrSays(text string) error {
	text = strings.ReplaceAll(strings.ReplaceAll(text, `\"`, `"`), "…", "...")
	if strings.Contains(text, "<message>") || strings.Contains(text, "<fix>") {
		return w.saysEachProblem(text)
	}
	if said, names, ok := strings.Cut(text, `" and names `); ok {
		return w.saysNaming(said, names)
	}
	for _, o := range w.outcomes() {
		if err := w.saysIn(o.stderr, text); err != nil {
			return fmt.Errorf("%v\n--- the run of itos-cc %s, exit %d\n--- stdout\n%s--- stderr\n%s", err, strings.Join(o.run.args, " "), o.exit, o.stdout, o.stderr)
		}
	}
	return nil
}

// saysNaming is whether a line of each run's stderr begins with said and
// names each of names, a list such as `a, "b" and "c"`, each with or
// without its quotes.
func (w *world) saysNaming(said, names string) error {
	var each []string
	for _, part := range strings.Split(strings.ReplaceAll(names, " and ", ", "), ", ") {
		each = append(each, strings.Trim(part, `"`))
	}
	for _, o := range w.outcomes() {
		if !slices.ContainsFunc(outputLines(filepath.ToSlash(o.stderr)), func(line string) bool {
			return strings.HasPrefix(line, said) && !slices.ContainsFunc(each, func(name string) bool { return !strings.Contains(line, name) })
		}) {
			return fmt.Errorf("no line of stderr begins %q and names %q\n--- the run of itos-cc %s, exit %d\n--- stdout\n%s--- stderr\n%s",
				said, each, strings.Join(o.run.args, " "), o.exit, o.stdout, o.stderr)
		}
	}
	return nil
}

// saysEachProblem is whether the last run reported a problem, and its
// stderr says text of each, <message> and <fix> in it standing for the
// problem's, as the run made with --json gives them (lastObject).
func (w *world) saysEachProblem(text string) error {
	object, report, err := w.lastObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	if len(problems) == 0 {
		return fmt.Errorf("the run reported no problem\n%s", report)
	}
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		message, _ := problem["message"].(string)
		fix, _ := problem["fix"].(string)
		want := strings.ReplaceAll(strings.ReplaceAll(text, "<message>", message), "<fix>", fix)
		if message == "" || fix == "" || !strings.Contains(w.stderr, want) {
			return fmt.Errorf("stderr does not say %q\n%s", want, report)
		}
	}
	return nil
}

// saysIn is whether stderr, a run's, holds text, read as stderrSays reads
// it.
func (w *world) saysIn(stderr, text string) error {
	if !strings.Contains(text, "<dir>") && !strings.Contains(text, "...") {
		// A path is named in the system's form, which a scenario writes
		// with slashes.
		if !strings.Contains(stderr, text) && !strings.Contains(filepath.ToSlash(stderr), text) {
			return fmt.Errorf("stderr does not say %q", text)
		}
		return nil
	}
	if strings.Contains(text, "<dir>") && w.project == "" {
		return fmt.Errorf("%q names <dir>, and no Given made a project's folder", text)
	}
	var pattern strings.Builder
	for i, part := range strings.Split(text, "...") {
		if i > 0 {
			pattern.WriteString(".+")
		}
		pieces := strings.Split(part, "<dir>")
		for j, piece := range pieces {
			if j > 0 {
				pattern.WriteString(regexp.QuoteMeta(w.project))
			}
			pattern.WriteString(regexp.QuoteMeta(piece))
		}
	}
	if !regexp.MustCompile(pattern.String()).MatchString(filepath.ToSlash(stderr)) {
		return fmt.Errorf("stderr does not say %q, <dir> being %s", text, w.project)
	}
	return nil
}

func outputLines(output string) []string {
	return strings.Split(strings.TrimSuffix(output, "\n"), "\n")
}

// exitsWith is whether each run the When made exited with code.
func (w *world) exitsWith(code int) error {
	for _, o := range w.outcomes() {
		if o.exit != code {
			return fmt.Errorf("itos-cc %s exited %d, not %d\n--- stdout\n%s--- stderr\n%s", strings.Join(o.run.args, " "), o.exit, code, o.stdout, o.stderr)
		}
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

// lastObject is the object of the last run, made again with --json before
// its words, as a script would make it, unless it had --json already: that
// run must exit as the plain one did. The report is both runs'.
func (w *world) lastObject() (map[string]any, string, error) {
	if w.last == nil {
		return nil, "", errors.New("no run of itos-cc to read")
	}
	stdout, report := w.stdout, w.report()
	if !slices.Contains(w.last.args, "--json") {
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, append([]string{"--json"}, w.last.args...)...)
		if err != nil {
			return nil, "", err
		}
		report = fmt.Sprintf("%s\n--- with --json, exit %d\n--- stdout\n%s--- stderr\n%s", report, code, out, errOut)
		if code != w.exit {
			return nil, "", fmt.Errorf("with --json itos-cc exited %d, not %d as without it\n%s", code, w.exit, report)
		}
		stdout = out
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return nil, "", fmt.Errorf("the output with --json is not one JSON object: %v\n%s", err, report)
	}
	return object, report, nil
}

// problemsAre is whether the --json object of the last run reports exactly
// the problems of the table, one a row under its header, by their rule and
// exit, in any order, a row twice for a rule reported twice. A run the
// steps started and stopped themselves, with no last run of its own, is
// read from its standard output; any other as lastObject reads it.
func (w *world) problemsAre(table *godog.Table) error {
	object, report, err := w.runObject()
	if err != nil {
		return err
	}
	var want []string
	for i, row := range table.Rows {
		if len(row.Cells) != 2 {
			return fmt.Errorf("row %d of the problems gives %d cells, not a rule and an exit", i+1, len(row.Cells))
		}
		if i == 0 {
			if row.Cells[0].Value != "rule" || row.Cells[1].Value != "exit" {
				return errors.New("the problems' header is not | rule | exit |")
			}
			continue
		}
		want = append(want, row.Cells[0].Value+" "+row.Cells[1].Value)
	}
	var got []string
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		p, _ := one.(map[string]any)
		exit := "none"
		if code, ok := p["exit"].(float64); ok {
			exit = fmt.Sprint(code)
		}
		got = append(got, fmt.Sprintf("%v %s", p["rule"], exit))
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("the problems by rule and exit are %q, not %q\n%s", got, want, report)
	}
	return nil
}

// stderrSaysOnce is whether exactly one line of stderr begins with text.
func (w *world) stderrSaysOnce(text string) error {
	n := 0
	for _, line := range outputLines(w.stderr) {
		if strings.HasPrefix(line, text) {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("%d lines of stderr begin %q, not one\n%s", n, text, w.report())
	}
	return nil
}

// endSignals are the signals a run ends by, as a scenario names its end,
// by their numbers on Linux and macOS: SIGINT and SIGTERM.
var endSignals = map[string]int{"interrupted": 2, "terminated": 15}

// endsAs is whether the last run, which a step signalled, reported its
// interrupt first, on stderr or in its --json object, and then ended as
// how says, which a shell reads as status, 128 and the signal's number: on
// Linux and macOS by that signal's default action, the one end a shell
// stops its loop for, and on Windows, where no process ends so, with exit
// 130. A run that ended by the signal without reporting never handled it.
func (w *world) endsAs(how string, status int) error {
	if want := 128 + endSignals[how]; status != want {
		return fmt.Errorf("a shell reads %d for a command %s, not %d", want, how, status)
	}
	if !w.reportedInterrupt() {
		return fmt.Errorf("itos-cc ended without reporting the interrupt\n%s", w.report())
	}
	if w.d.Platform() == "windows" {
		if w.exit != 130 {
			return fmt.Errorf("itos-cc exited %d, not 130\n%s", w.exit, w.report())
		}
		return nil
	}
	if w.signal != endSignals[how] {
		return fmt.Errorf("itos-cc ended with exit %d and signal %d, not by signal %d, which a shell reads as %d\n%s", w.exit, w.signal, endSignals[how], status, w.report())
	}
	return nil
}

// reportedInterrupt is whether the last run said it was interrupted: a
// line of stderr beginning "itos-cc: interrupted", or a problem of rule
// interrupted in its --json object.
func (w *world) reportedInterrupt() bool {
	for _, line := range outputLines(w.stderr) {
		if strings.HasPrefix(line, "itos-cc: interrupted") {
			return true
		}
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return false
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == "interrupted" {
			return true
		}
	}
	return false
}

// runObject is the --json object of the last run: of standard output when
// the steps started the run themselves, with no last run, else as
// lastObject gives it.
func (w *world) runObject() (map[string]any, string, error) {
	if w.last == nil {
		object, err := w.json()
		return object, w.report(), err
	}
	return w.lastObject()
}

// problemIs is whether the last run reports a problem of that rule, whose
// key, a flag, an argument, a key of itos-cc.yaml or a file, its
// separators read as slashes, is value; any problem
// of the rule where key is "". The plain output names no rule, so the
// problem is read from the run made with --json (lastObject), and the plain
// one's error output must print its message: a run with --json prints
// none, so it is made again without --json, which must exit as it did.
func (w *world) problemIs(rule, key, value string) error {
	object, report, err := w.lastObject()
	if err != nil {
		return err
	}
	plain, err := w.plainError()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		problem, _ := one.(map[string]any)
		got := problem[key]
		if file, ok := got.(string); ok && key == "file" {
			// A file is named in the system's form, which a scenario
			// writes with slashes.
			got = filepath.ToSlash(file)
		}
		if problem["rule"] != rule || key != "" && got != value {
			continue
		}
		if message, _ := problem["message"].(string); message == "" || !strings.Contains(plain, message) {
			return fmt.Errorf("the error output does not print the problem's message %q\n%s", message, report)
		}
		w.problem = problem
		return nil
	}
	if key == "" {
		return fmt.Errorf("no problem has the rule %q\n%s", rule, report)
	}
	return fmt.Errorf("no problem has the rule %q and the %s %q\n%s", rule, key, value, report)
}

// plainError is the error output of the last run made without --json: its
// own, or, when it had --json, that of the same run made again without it,
// which must exit as it did.
func (w *world) plainError() (string, error) {
	if !slices.Contains(w.last.args, "--json") {
		return w.stderr, nil
	}
	args := slices.DeleteFunc(slices.Clone(w.last.args), func(arg string) bool { return arg == "--json" })
	code, _, errOut, err := w.capture(w.last.dir, w.last.env, args...)
	if err != nil {
		return "", err
	}
	if code != w.exit {
		return "", fmt.Errorf("without --json itos-cc exited %d, not %d as with it\n%s\n--- without --json\n%s", code, w.exit, w.report(), errOut)
	}
	return errOut, nil
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

// problemSays is whether the field, message or fix, of the problem
// problemIs found holds text.
func (w *world) problemSays(field, text string) error {
	if w.problem == nil {
		return errors.New("no problem found to read the " + field + " of")
	}
	if got, _ := w.problem[field].(string); !strings.Contains(got, text) {
		return fmt.Errorf("the problem's %s %q does not say %q\n%s", field, got, text, w.report())
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
	fileShape := strings.HasSuffix(list, ", each with file, killed, survived, uncovered, ran, reused, and baseline")
	if fileShape {
		list = strings.TrimSuffix(list, ", each with file, killed, survived, uncovered, ran, reused, and baseline")
	}

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
	if fileShape {
		files, ok := object["files"].([]any)
		if !ok || len(files) == 0 {
			return fmt.Errorf("no result files: %v", object)
		}
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("file is not an object: %v", raw)
			}
			for _, key := range []string{"file", "killed", "survived", "uncovered", "ran", "reused", "baseline"} {
				if file[key] == nil {
					return fmt.Errorf("file lacks %s: %v", key, file)
				}
			}
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

// stdoutSays checks literal text, or one line whose ordered pieces surround
// the scenario's ellipsis, as mutation summaries name only their suffix.
func (w *world) stdoutSays(text string) error {
	if !strings.Contains(text, "…") {
		if strings.Contains(w.stdout, text) || strings.Contains(filepath.ToSlash(w.stdout), text) {
			return nil
		}
	} else {
		for _, line := range outputLines(w.stdout) {
			rest := filepath.ToSlash(line)
			matched := true
			for _, part := range strings.Split(text, "…") {
				at := strings.Index(rest, part)
				if at < 0 {
					matched = false
					break
				}
				rest = rest[at+len(part):]
			}
			if matched {
				return nil
			}
		}
	}
	return fmt.Errorf("stdout does not say %q\n%s", text, w.report())
}
