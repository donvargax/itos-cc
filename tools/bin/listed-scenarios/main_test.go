package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

// one is a converted file's scenarios: one the driver runs, one only the
// binary runs, a deferred one, one @wip though the listing calls it live,
// an Unstepped one, an outline with one of its two Examples deferred and
// one with its only Examples deferred.
const one = `Feature: one

  @ID-ONE-01
  Scenario: runs in process
    Given a step

  @ID-ONE-02 @binary-only
  Scenario: the binary's alone
    Given a step

  @ID-ONE-03
  Scenario: deferred
    Given a step

  @ID-ONE-04 @wip
  Scenario: not written yet
    Given a step

  @ID-ONE-05
  Scenario: unstepped
    Given a step

  @ID-ONE-06
  Scenario Outline: some rows deferred
    Given <x>

    Examples:
      | x |
      | a |

    @T-9
    Examples:
      | x |
      | b |

  @ID-ONE-07
  Scenario Outline: every row deferred
    Given <x>

    @T-9
    Examples:
      | x |
      | c |
`

// two is a converted file the binary alone runs, by its feature's tag.
const two = `@binary-only
Feature: two

  @ID-TWO-01
  Scenario: the binary's by its feature
    Given a step
`

// fixture writes the feature files under a folder of its own and returns
// it: three.feature is never written, as no converted file names it.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"one.feature": one, "two.feature": two} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const listingJSON = `{"schema": 1, "tests": [
  {"id": "ID-ONE-07", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-01", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-02", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-03", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-04", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-05", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-06", "file": "one.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-ONE-08", "file": "one.feature", "live": false, "fingerprint": "f"},
  {"id": "ID-TWO-01", "file": "two.feature", "live": true, "fingerprint": "f"},
  {"id": "ID-THREE-01", "file": "three.feature", "live": true, "fingerprint": "f"}
]}`

var rules = harness{
	converted: []string{"one.feature", "two.feature"},
	skipped:   []string{"@wip", "@binary-only", "@ID-ONE-05", "@ID-ONE-03", "@T-9"},
}

func TestListsTheLiveScenariosTheDriverRuns(t *testing.T) {
	dir := fixture(t)
	got, err := listed(strings.NewReader(listingJSON), dir, rules)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.ToSlash(filepath.Join(dir, "one.feature"))
	want := []test{{"ID-ONE-01", file}, {"ID-ONE-06", file}}
	if !slices.Equal(got, want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
}

func TestPrintsEachLineAnIDATabAndItsFile(t *testing.T) {
	var out, stderr bytes.Buffer
	dir := fixture(t)
	if code := run([]string{"-dir", dir}, strings.NewReader(listingJSON), &out, &stderr, rules); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	file := filepath.ToSlash(filepath.Join(dir, "one.feature"))
	if want := "ID-ONE-01\t" + file + "\nID-ONE-06\t" + file + "\n"; out.String() != want || stderr.Len() != 0 {
		t.Fatalf("printed %q and %q, want %q", out.String(), stderr.String(), want)
	}
}

func TestMainListsWithTheHarnessRules(t *testing.T) {
	dir := t.TempDir()
	runs, binary := "units.feature", "mutate.feature"
	if !slices.Contains(features.Converted, runs) || !slices.Contains(features.Converted, binary) {
		t.Fatalf("features.Converted %v lacks %s or %s", features.Converted, runs, binary)
	}
	if err := os.WriteFile(filepath.Join(dir, runs), []byte("Feature: units\n\n  @ID-A-01\n  Scenario: a\n    Given a step\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "Feature: mutate\n\n  @ID-B-01 " + features.BinaryOnly + "\n  Scenario: b\n    Given a step\n\n" +
		"  @ID-C-01 " + strings.Join(slices.Sorted(maps.Keys(features.Deferred)), " ") + "\n  Scenario: c\n    Given a step\n\n" +
		"  @ID-D-01 @wip " + strings.Join(features.Unstepped, " ") + "\n  Scenario: d\n    Given a step\n"
	if err := os.WriteFile(filepath.Join(dir, binary), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "listing.json")
	list := `{"tests": [{"id": "ID-A-01", "file": "units.feature", "live": true}, {"id": "ID-B-01", "file": "mutate.feature", "live": true}` +
		`, {"id": "ID-C-01", "file": "mutate.feature", "live": true}, {"id": "ID-D-01", "file": "mutate.feature", "live": true}]}`
	if err := os.WriteFile(in, []byte(list), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(in) //nolint:gosec // the test's own temporary file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	outPath := filepath.Join(dir, "out")
	stdout, err := os.Create(outPath) //nolint:gosec // the test's own temporary file
	if err != nil {
		t.Fatal(err)
	}
	args, oldIn, oldOut, oldExit := os.Args, os.Stdin, os.Stdout, exit
	defer func() { os.Args, os.Stdin, os.Stdout, exit = args, oldIn, oldOut, oldExit }()
	code := -1
	os.Args, os.Stdin, os.Stdout, exit = []string{self, "-dir", dir}, stdin, stdout, func(c int) { code = c }
	main()
	os.Stdout = oldOut
	_ = stdout.Close()
	printed, err := os.ReadFile(outPath) //nolint:gosec // the test's own temporary file
	if err != nil {
		t.Fatal(err)
	}
	if want := "ID-A-01\t" + filepath.ToSlash(filepath.Join(dir, runs)) + "\n"; code != 0 || string(printed) != want {
		t.Fatalf("exit %d, printed %q, want %q", code, printed, want)
	}
}

func TestRefusesWhatItCannotRead(t *testing.T) {
	dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, "bad.feature"), []byte("not a keyword line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := harness{converted: []string{"bad.feature", "gone.feature"}}
	for name, c := range map[string]struct {
		listing string
		h       harness
		want    string
	}{
		"empty":        {"", rules, "reading itos tests list scenario --json: EOF"},
		"not JSON":     {"oops", rules, "reading itos tests list scenario --json: invalid character"},
		"no tests":     {`{"ok": false}`, rules, "reading itos tests list scenario --json: no tests array"},
		"missing file": {`{"tests": [{"id": "ID-G-01", "file": "gone.feature", "live": true}]}`, bad, "gone.feature"},
		"bad gherkin":  {`{"tests": [{"id": "ID-B-01", "file": "bad.feature", "live": true}]}`, bad, "bad.feature:"},
	} {
		t.Run(name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			if code := run([]string{"-dir", dir}, strings.NewReader(c.listing), &out, &stderr, c.h); code != 1 || out.Len() != 0 ||
				!strings.HasPrefix(stderr.String(), self+": ") || !strings.Contains(stderr.String(), c.want) {
				t.Fatalf("exit %d, printed %q and %q, want 1 and %q", code, out.String(), stderr.String(), c.want)
			}
		})
	}
}

func TestReadsEachFeatureFileOnce(t *testing.T) {
	dir := fixture(t)
	h := harness{converted: []string{"one.feature"}}
	listing := `{"tests": [{"id": "ID-ONE-01", "file": "one.feature", "live": true}, {"id": "ID-ONE-02", "file": "one.feature", "live": true}]}`
	reads := 0
	h.read = func(path string) ([]byte, error) {
		reads++
		return os.ReadFile(path) //nolint:gosec // the test's own temporary file
	}
	got, err := listed(strings.NewReader(listing), dir, h)
	if err != nil || len(got) != 2 || reads != 1 {
		t.Fatalf("listed %v, %v, reading %d times, want 2 tests from one read", got, err, reads)
	}
}

func TestRefusesAUsageError(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"-nope"}} {
		var out, stderr bytes.Buffer
		if code := run(args, strings.NewReader(listingJSON), &out, &stderr, rules); code != 2 || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("%v: exit %d, printed %q and %q, want 2 and a usage line", args, code, out.String(), stderr.String())
		}
	}
}
