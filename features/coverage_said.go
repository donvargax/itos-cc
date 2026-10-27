package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Reading what itos-cc said and printed while it measured coverage, and where
// its coverage starts.

// covAnnouncement is a command itos-cc announced it runs to measure
// coverage: the folder it runs in, slash-separated from the scratch folder,
// "" for --coverage-command's, and its words.
type covAnnouncement struct {
	dir, command string
	words        []string
}

// announced are the coverage commands stderr announces.
func (c *coverageState) announced(stderr string) []covAnnouncement {
	var out []covAnnouncement
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		rest, ok := strings.CutPrefix(line, "itos-cc: coverage ")
		if !ok {
			continue
		}
		dir, command, ok := strings.Cut(rest, "$ ")
		if !ok {
			continue
		}
		a := covAnnouncement{command: command, words: strings.Fields(command)}
		if dir = strings.TrimSpace(dir); dir != "" {
			if a.dir, ok = c.rel(dir); !ok {
				a.dir = "?" + dir
			}
		}
		out = append(out, a)
	}
	return out
}

// rel is p, a path itos-cc printed, slash-separated from the scratch folder:
// found by the scratch folder's name, as the system may name the folder
// through another path (/private/var for /var, a short name on windows);
// false when p is not in it.
func (c *coverageState) rel(p string) (string, bool) {
	p = filepath.ToSlash(strings.Trim(p, `"`))
	base := "/" + filepath.Base(c.w.dir)
	i := strings.LastIndex(p, base)
	if i < 0 {
		return "", false
	}
	switch rest := p[i+len(base):]; {
	case rest == "":
		return ".", true
	case strings.HasPrefix(rest, "/"):
		return rest[1:], true
	}
	return "", false
}

// program is a command's program as its first word names it, without the
// extension windows runs it by.
func program(word string) string {
	base := path.Base(filepath.ToSlash(strings.Trim(word, `"`)))
	for _, ext := range []string{".cmd", ".exe", ".bat"} {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

// commandPattern is a command as a scenario writes it, "..." standing for
// any words, or any text within a word, as a regular expression over the
// whole command.
func commandPattern(text string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i, token := range strings.Fields(text) {
		if token == "..." {
			b.WriteString(`(?: \S+)*`)
			continue
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(token), `\.\.\.`, ".*"))
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// wordPattern is a word as a scenario writes it, "..." standing for any
// text.
func wordPattern(word string) *regexp.Regexp {
	return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(word), `\.\.\.`, ".*") + "$")
}

// listed is what the last run listed, from its object or its table.
func (c *coverageState) listed() ([]crapEntry, error) {
	return (&crapState{w: c.w}).listed()
}

// coverageOf is the coverage the entries give f, nil for none; an error
// when they list no such function.
func coverageOf(entries []crapEntry, f covFunction) (*float64, error) {
	for _, e := range entries {
		if filepath.ToSlash(e.File) == f.file && e.Name == f.name {
			return e.Coverage, nil
		}
	}
	return nil, fmt.Errorf("%s of %s is not listed", f.name, f.file)
}

// hasCoverage is whether the entries give f want, as crap shows it.
func (c *coverageState) hasCoverage(entries []crapEntry, f covFunction, want float64) error {
	got, err := coverageOf(entries, f)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, c.w.report())
	}
	if shown(got) != shown(&want) {
		return fmt.Errorf("%s of %s has %s%% coverage, not %s%%\n%s", f.name, f.file, shown(got), shown(&want), c.w.report())
	}
	return nil
}

// Where coverage starts.

// startsAt is whether the function "the <language> function:" wrote, its
// first line first when that is given, is measured from line from: a
// report that ran that line alone of every line of it gives it the share
// of one line of those from there to its end.
func (c *coverageState) startsAt(from int, firstLine string) error {
	first, _ := strconv.Atoi(firstLine)
	file := c.w.wrote
	if file == "" {
		return errors.New("no function was written")
	}
	code, stdout, stderr, err := c.w.capture(c.w.dir, c.w.env(), "crap", "--no-coverage", "--json", file)
	if err != nil {
		return err
	}
	var object crapObject
	if err := json.Unmarshal([]byte(stdout), &object); err != nil || code != 0 || len(object.Entries) != 1 {
		return fmt.Errorf("the function could not be measured: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	e := object.Entries[0]
	switch {
	case first != 0 && e.StartLine != first:
		return fmt.Errorf("its first line is %d, not %d", e.StartLine, first)
	case from < e.StartLine || from > e.EndLine:
		return fmt.Errorf("line %d is not of the function, lines %d to %d", from, e.StartLine, e.EndLine)
	}
	var lines [][2]int
	for line := e.StartLine; line <= e.EndLine; line++ {
		lines = append(lines, [2]int{line, hits(line == from)})
	}
	if err := c.w.write("probe.info", lcovRecord(file, lcovLines(lines...), nil)); err != nil {
		return err
	}
	if err := c.w.runEnv(c.w.dir, c.w.env(), "crap", "--json", "--coverage-report", "probe.info", file); err != nil {
		return err
	}
	entries, err := c.listed()
	if err != nil {
		return err
	}
	return c.hasCoverage(entries, covFunction{file, e.Name}, 100/float64(e.EndLine-from+1))
}

// What itos-cc says while it measures.

// announcesCommand is whether stderr has a line of text, <dir> standing for
// the build root of the sources the run named and … for the rest of the
// line, the command it names the one that ran: its output follows.
func (c *coverageState) announcesCommand(text string) error {
	before, after, ok := strings.Cut(text, "<dir>")
	if !ok {
		return fmt.Errorf("%q names no <dir>", text)
	}
	after, rest := strings.CutSuffix(after, "…")
	re := regexp.MustCompile("^" + regexp.QuoteMeta(before) + "(.+?)" + regexp.QuoteMeta(after) + map[bool]string{true: ".*", false: ""}[rest] + "$")
	roots, err := c.selectedRoots()
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n")
	for i, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if dir, in := c.rel(m[1]); !in || !slices.Contains(roots, dir) {
			return fmt.Errorf("%q names %s, not the build root of the run's sources, %v\n%s", line, m[1], roots, c.w.report())
		}
		if i+1 == len(lines) || !strings.HasPrefix(lines[i+1], "ok ") {
			return fmt.Errorf("the command %q names did not run\n%s", line, c.w.report())
		}
		return nil
	}
	return fmt.Errorf("stderr has no line %q\n%s", text, c.w.report())
}

// noLineBegins is whether no line of stderr begins with text.
func (c *coverageState) noLineBegins(text string) error {
	for _, line := range strings.Split(c.w.stderr, "\n") {
		if strings.HasPrefix(line, text) {
			return fmt.Errorf("stderr has the line %q\n%s", line, c.w.report())
		}
	}
	return nil
}

// saysLine is whether a line of stderr is text, or begins with it when
// beginning, and holds names; each <dir> in text stands for the build root
// of the sources the run named, as itos-cc names it, each <duration> for a
// duration's shape (durationShape), and a path is read slash-separated, as
// a scenario writes it.
func (c *coverageState) saysLine(text, names string, beginning bool) error {
	var b strings.Builder
	b.WriteString("^")
	for i, piece := range strings.Split(text, "<dir>") {
		if i > 0 {
			b.WriteString("(.+?)")
		}
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(piece), "<duration>", durationShape))
	}
	if !beginning {
		b.WriteString("$")
	}
	re := regexp.MustCompile(b.String())
	roots, err := c.selectedRoots()
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n") {
		line = filepath.ToSlash(line)
		m := re.FindStringSubmatch(line)
		if m == nil || !strings.Contains(line, names) {
			continue
		}
		for _, dir := range m[1:] {
			if rel, in := c.rel(dir); !in || !slices.Contains(roots, rel) {
				return fmt.Errorf("%q names %s, not the build root of the run's sources, %v\n%s", line, dir, roots, c.w.report())
			}
		}
		return nil
	}
	if names != "" {
		return fmt.Errorf("no line of stderr begins %q and names %q\n%s", text, names, c.w.report())
	}
	return fmt.Errorf("stderr has no line %q\n%s", text, c.w.report())
}

// saysNowhere is whether no line of the stderr of any run the When made
// holds text.
func (c *coverageState) saysNowhere(text string) error {
	for _, o := range c.w.outcomes() {
		for _, line := range strings.Split(o.stderr, "\n") {
			if strings.Contains(line, text) {
				return fmt.Errorf("the run of itos-cc %s says %q\n--- stderr\n%s", strings.Join(o.run.args, " "), line, o.stderr)
			}
		}
	}
	return nil
}

// exitStatusLine is whether a line of stderr begins with text and gives the
// exit status the failing command exited with.
func (c *coverageState) exitStatusLine(text string) error {
	re := regexp.MustCompile(`exit status [1-9]\d*$`)
	for _, line := range strings.Split(strings.ReplaceAll(c.w.stderr, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, text) && re.MatchString(line) {
			return nil
		}
	}
	return fmt.Errorf("no line of stderr begins %q and gives an exit status\n%s", text, c.w.report())
}
