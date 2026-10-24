package features

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Sources itos-cc.yaml excludes.

// The Go module of "Rule: Sources itos-cc.yaml excludes are not mutated":
// src/board.go's place, which TestPlace kills, and clear, which only
// TestClear, where a scenario has it, reaches, beside a fake in a package
// of its own, which no test reaches. Line 7 of the fake holds three sites,
// the first the 0 at column 9.
const (
	boardGo = `package board

type Board struct{}

func (b *Board) place(i int) bool {
	// past five
	return i > 5
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`
	placeTest = `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`
	clearTest = `
func TestClear(t *testing.T) {
	var b Board
	if !b.clear(2) || b.clear(3) {
		t.Fatal("clear")
	}
}
`
	fakeGo = `package porttest

// Fake stands in for a board in tests.
type Fake struct{ n int }

func (f *Fake) Over(i int) bool {
	return 0 < i-f.n
}
`
	// fakeException excepts the fake's survivor at line 7, column 9, as
	// a person or mutation except writes one.
	fakeException = `mutation:
  exceptions:
    - file: %s
      function: example.com/m/src/port/porttest.Fake#Over
      hash: "0000000000000000"
      line_in_function: 2
      column: 9
      original: "0"
      replacement: "1"
      reason: the fake's bound is never read
`
)

// boardModule writes the module with the fake at fake, its tests killing
// clear's mutant too where both, and declares what they do and cover.
func (m *mutateState) boardModule(fake string, both bool) error {
	tests, report := placeTest, "SF:src/board.go\nDA:7,1\nDA:11,0\nend_of_record\n"
	if both {
		tests, report = placeTest+clearTest, "SF:src/board.go\nDA:7,1\nDA:11,1\nend_of_record\n"
	}
	if !strings.HasSuffix(fake, "/porttest/fake.go") {
		return fmt.Errorf("the fake is %s, not a porttest package's fake.go", fake)
	}
	if err := m.writeAll(map[string]string{
		"go.mod": mutateModule, "src/board.go": boardGo, "src/board_test.go": tests, fake: fakeGo,
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "go", Report: report}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, goTests)
}

// onlyCommitChanges writes the module beside the itos-cc.yaml a Given
// wrote, with the results of a run on main, the fake's mutants reached by
// no test, then commits a change to the fake alone on a branch of main.
func (m *mutateState) onlyCommitChanges(fake string) error {
	if m.read("itos-cc.yaml") == "" {
		return errors.New("no itos-cc.yaml was written before the module")
	}
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if strings.Contains(m.read("src/board_test.go"), "porttest") {
		return errors.New("a test reaches the fake")
	}
	if err := m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), "mutation", "run", "--no-annotate"); err != nil || code != 0 {
		return fmt.Errorf("the run on main failed: exit %d, %v\n%s%s", code, err, stdout, stderr)
	}
	for _, args := range [][]string{
		{"add", "-A"}, {"commit", "-qm", "base"}, {"checkout", "-qb", "work"},
	} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write(fake, strings.Replace(fakeGo, "0 < i-f.n", "1 < i-f.n", 1)); err != nil {
		return err
	}
	if err := m.w.git("add", "-A"); err != nil {
		return err
	}
	return m.w.git("commit", "-qm", "change the fake")
}

// siteMention is a site a command names: its file and line, and its column
// where it gives one, before the text a mutant replaces.
var siteMention = regexp.MustCompile("(\\S+):\\d+(?::\\d+)? `")

// neitherNames is whether each command names a site, and none in file,
// whose name neither says at all.
func (m *mutateState) neitherNames(first, second, file string) error {
	for _, args := range []string{first, second} {
		words, err := commandWords(args)
		if err != nil {
			return err
		}
		code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), words...)
		if err != nil {
			return err
		}
		output := filepath.ToSlash(stdout + stderr)
		mentions := siteMention.FindAllStringSubmatch(output, -1)
		if len(mentions) == 0 || strings.Contains(output, path.Base(file)) {
			return fmt.Errorf("itos-cc %s names no site, or names %s: exit %d\n%s", args, path.Base(file), code, output)
		}
	}
	return nil
}

// excludedSnapshot writes the module in a git repository and records a
// run's results of every source, the fake's at fake among them, all fresh.
func (m *mutateState) excludedSnapshot(fake string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"mutation", "run"}, {"mutation", "check"}} {
		if code, stdout, stderr, err := m.w.capture(m.w.dir, m.w.env(), args...); err != nil || code != 0 {
			return fmt.Errorf("itos-cc %s: exit %d, %v\n%s%s", strings.Join(args, " "), code, err, stdout, stderr)
		}
	}
	m.source, m.snapshot = fake, ".metrics/mutate/"+fake+".json"
	if !m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("the run recorded no %s", m.snapshot)
	}
	return nil
}

// lists is whether the last run's --json object's key lists an entry of
// file with name value.
func (m *mutateState) lists(key, file, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	entries, _ := object[key].([]any)
	for _, one := range entries {
		entry, _ := one.(map[string]any)
		if got, _ := entry["file"].(string); filepath.ToSlash(got) == file {
			if entry[name] != value {
				return fmt.Errorf("%q lists %s with %q %v, not %q\n%s", key, file, name, entry[name], value, m.w.report())
			}
			return nil
		}
	}
	return fmt.Errorf("%q does not list %s\n%s", key, file, m.w.report())
}

// snapshotRemoved is whether the snapshot the Given recorded is gone, and
// the last run's key lists its source with name value.
func (m *mutateState) snapshotRemoved(key, name, value string) error {
	if m.snapshot == "" {
		return errors.New("no Given recorded a snapshot")
	}
	if m.w.d.Exists(filepath.Join(m.w.dir, filepath.FromSlash(m.snapshot))) {
		return fmt.Errorf("%s is still there\n%s", m.snapshot, m.w.report())
	}
	return m.lists(key, m.source, name, value)
}

// exceptsAndExcludes writes the module with the fake at fake, and an
// itos-cc.yaml excluding glob and excepting a survivor of the fake.
func (m *mutateState) exceptsAndExcludes(fake, glob string) error {
	if err := m.boardModule(fake, true); err != nil {
		return err
	}
	return m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"+fmt.Sprintf(fakeException, fake))
}

// reports is whether the last run's --json object reports a problem of
// rule with name value.
func (m *mutateState) reports(rule, name, value string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		if p, _ := one.(map[string]any); p["rule"] == rule && p[name] == value {
			return nil
		}
	}
	return fmt.Errorf("no problem is %s with %q %q\n%s", rule, name, value, m.w.report())
}

// headExcludes commits the module with an itos-cc.yaml excluding glob,
// then leaves one excluding nothing in the working tree, where mutation
// list lists the fake's sites: only the file HEAD commits leaves them out.
func (m *mutateState) headExcludes(glob string) error {
	if err := m.w.d.Counted(); err != nil {
		return err
	}
	fake := "src/port/porttest/fake.go"
	if err := m.boardModule(fake, false); err != nil {
		return err
	}
	if err := m.w.write("itos-cc.yaml", "exclude: ["+strconv.Quote(glob)+"]\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	if err := m.w.write("itos-cc.yaml", "exclude: []\n"); err != nil {
		return err
	}
	sites, err := m.sitesOf(".")
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(sites, func(s listedSite) bool { return s.file == fake }) {
		return fmt.Errorf("mutation list lists no site of %s: %v", fake, sites)
	}
	return nil
}

// headHolds commits the module with an itos-cc.yaml holding text, then
// leaves one excluding nothing in the working tree, so a refusal of text
// comes from the file HEAD commits, which a counted run reads.
func (m *mutateState) headHolds(text string) error {
	if err := m.w.d.Counted(); err != nil {
		return err
	}
	if err := m.boardModule("src/port/porttest/fake.go", false); err != nil {
		return err
	}
	if err := m.w.write("itos-cc.yaml", text+"\n"); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		if err := m.w.git(args...); err != nil {
			return err
		}
	}
	return m.w.write("itos-cc.yaml", "exclude: []\n")
}

// fixSaysThen is whether the fix of the problem problemIs found opens
// with first and says then after it, and the error output prints it.
func (m *mutateState) fixSaysThen(first, then string) error {
	if m.w.problem == nil {
		return errors.New("no problem found to read the fix of")
	}
	fix, _ := m.w.problem["fix"].(string)
	rest, opens := strings.CutPrefix(fix, first)
	if !opens || !strings.Contains(rest, then) {
		return fmt.Errorf("the problem's fix %q does not open with %q and say %q after it\n%s", fix, first, then, m.w.report())
	}
	if !strings.Contains(m.w.stderr, fix) {
		return fmt.Errorf("the error output does not print the problem's fix %q\n%s", fix, m.w.report())
	}
	return nil
}

// noSelectedSiteIn is whether the last run's --json object selects sites,
// none in file.
func (m *mutateState) noSelectedSiteIn(file string) error {
	object, err := m.w.json()
	if err != nil {
		return err
	}
	selected, _ := object["selected"].([]any)
	if len(selected) == 0 {
		return fmt.Errorf("the run selected no site\n%s", m.w.report())
	}
	for _, one := range selected {
		site, _ := one.(map[string]any)
		if got, _ := site["file"].(string); filepath.ToSlash(got) == file {
			return fmt.Errorf("the run selected %v, in %s\n%s", site, file, m.w.report())
		}
	}
	return nil
}
