package settings

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestAMissingFileIsAnEmptyConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	c, err := Load()
	if err != nil || len(c.Exceptions) != 0 {
		t.Errorf("config %+v, error %v, want none of either", c, err)
	}
}

func TestAnInvalidFileIsAnInvalidError(t *testing.T) {
	for text, want := range map[string]string{
		"mutation: [\n": "",
		"- a\n":         "",
		"mutation: 5\n": "",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', replacement: '<='}\n":              "mutation.exceptions[0] has no reason",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 0, column: 1, original: '<', replacement: '<=', reason: ' '}\n": "has no line_in_function of 1 or more, no reason",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', reason: r}\n":                      "has no replacement",
	} {
		t.Chdir(t.TempDir())
		if err := os.WriteFile(File, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load()
		var invalid *InvalidError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an InvalidError saying %q", text, err, want)
		}
	}
}

const testsYAML = `mutation:
  tests:
    list: ./list
    run: go test ./e2e -run '{pattern}'
    ids_pattern: "^Test({ids})$"
    join:
      each: "{id}"
      sep: "|"
`

func TestMutationTestsSelectTheirIDs(t *testing.T) {
	c, err := Parse([]byte(testsYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.Tests == nil || c.Tests.List != "./list" {
		t.Fatalf("tests %+v, want those mutation.tests names", c.Tests)
	}
	if got, want := c.Tests.Select([]string{"A", "B"}), "go test ./e2e -run '^Test(A|B)$'"; got != want {
		t.Errorf("Select(A, B) = %q, want %q", got, want)
	}
	if got, want := c.Tests.All([]string{"A"}), "go test ./e2e -run '^Test(A)$'"; got != want {
		t.Errorf("All(A) = %q, want %q: run with every ID, without whole", got, want)
	}
	c, err = Parse([]byte(testsYAML + "    whole: go test ./e2e\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Tests.All([]string{"A"}); got != "go test ./e2e" {
		t.Errorf("All(A) = %q, want whole", got)
	}
	c, err = Parse([]byte(testsYAML + "    support: [\"features/*_test.go\"]\n"))
	if err != nil || !slices.Equal(c.Tests.Support, []string{"features/*_test.go"}) {
		t.Errorf("support %v, %v, want the glob", c, err)
	}
	if c, err := Parse([]byte("mutation: {}\n")); err != nil || c.Tests != nil {
		t.Errorf("without mutation.tests: %+v, %v, want no tests", c, err)
	}
}

func TestMalformedMutationTestsAreInvalid(t *testing.T) {
	for _, c := range []struct{ from, to, says string }{
		{"    list: ./list\n", "", "has no list"},
		{"'{pattern}'", "x", "has a run without {pattern}"},
		{"({ids})", "(x)", "has an ids_pattern without {ids}"},
		{`      each: "{id}"`, `      each: "x"`, "has a join.each without {id}"},
		{"      sep: \"|\"\n", "", "has no join.sep"},
		{"    join:\n      each: \"{id}\"\n      sep: \"|\"\n", "", "has no join"},
		{"    list: ./list\n", "    list: ./list\n    support: [\"features/[\"]\n", "has a support glob \"features/[\" that is no pattern"},
	} {
		_, err := Parse([]byte(strings.Replace(testsYAML, c.from, c.to, 1)))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("without %q: error %v, want an *InvalidError that says %q", c.from, err, c.says)
		}
	}
}

// exclude holds doublestar globs from the project root, in order, a glob
// that matches nothing among them.
func TestExcludeReadsItsGlobs(t *testing.T) {
	c, err := Parse([]byte("exclude: [\"**/porttest/**\", \"features/*.go\", \"{a,b}/x?.[ch]\", \"nothing/here\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"**/porttest/**", "features/*.go", "{a,b}/x?.[ch]", "nothing/here"}; !slices.Equal(c.Selection.Exclude, want) {
		t.Errorf("exclude %q, want %q", c.Selection.Exclude, want)
	}
}

// A glob that is not one from the project root is refused, naming its key,
// before anything else is read.
func TestExcludeRefusesWhatIsNoPatternFromTheRoot(t *testing.T) {
	for _, c := range []struct{ glob, why string }{
		{"", "is empty"},
		{"  ", "is empty"},
		{"/src/**", "is absolute"},
		{"C:/src/**", "is absolute"},
		{"../other/**", "leaves the project root"},
		{"src/../../x", "leaves the project root"},
		{`src\port\**`, "backslash"},
		{"src/[x", "is no pattern"},
		{"src/{a,b", "is no pattern"},
	} {
		text := "exclude:\n  - src/**\n  - '" + c.glob + "'\nmutation:\n  exceptions:\n    - {file: x}\n"
		_, err := Parse([]byte(text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%q: error %v, want an *InvalidError", c.glob, err)
			continue
		}
		if invalid.Key != "exclude[1]" || !strings.Contains(invalid.Reason, "exclude[1] ") || !strings.Contains(invalid.Reason, c.why) {
			t.Errorf("%q: key %q, reason %q, want exclude[1] and %q", c.glob, invalid.Key, invalid.Reason, c.why)
		}
	}
}

// An exclude that is no list is a file that cannot be read.
func TestExcludeThatIsNoListIsInvalid(t *testing.T) {
	var invalid *InvalidError
	if _, err := Parse([]byte("exclude: {a: b}\n")); !errors.As(err, &invalid) {
		t.Errorf("error %v, want an *InvalidError", err)
	}
}

// An anchor, an alias or a merge key anywhere in the file is refused,
// before anything else is read, at the key whose value carries the first
// anchor in document order, or at the merge key itself, with a fix of its
// own (docs/CONFIG.md rule 1, bug-8).
func TestAnchorsAliasesAndMergeKeysAreRefused(t *testing.T) {
	for _, c := range []struct{ text, key, found string }{
		{"exclude: &e [\"**/porttest/**\"]\n", "exclude", "exclude carries the anchor &e"},
		{"exclude: [&g \"**/porttest/**\", *g]\n", "exclude[0]", "exclude[0] carries the anchor &g"},
		{"{x: &e [\"**/porttest/**\"], exclude: *e}\n", "x", "x carries the anchor &e"},
		{"{x: &e [], mutation: {exceptions: *e}}\n", "x", "x carries the anchor &e"},
		{"mutation: {<<: {exceptions: []}}\n", "mutation.<<", "mutation.<< is a merge key"},
		{"<<: {exclude: []}\n", "<<", "<< is a merge key"},
		{"&k exclude: []\n", "exclude", "exclude carries the anchor &k"},
		{"&r {exclude: []}\n", "", "the top level carries the anchor &r"},
		{"mutation:\n  exceptions:\n    - {file: &f a.go}\n", "mutation.exceptions[0].file", "mutation.exceptions[0].file carries the anchor &f"},
		{"exclude: ['', &g a]\n", "exclude[1]", "exclude[1] carries the anchor &g"},
		{"exclude: [a]\nmutation: [&m b]\n", "mutation[0]", "mutation[0] carries the anchor &m"},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%q: error %v, want an *InvalidError", c.text, err)
			continue
		}
		want := "itos-cc.yaml: " + c.found + ", and itos-cc.yaml takes no anchors, aliases or merge keys"
		if invalid.Key != c.key || err.Error() != want || invalid.Fix != spellingFix {
			t.Errorf("%q: key %q, error %q, fix %q; want %q, %q and the fix to write the value in place", c.text, invalid.Key, err, invalid.Fix, c.key, want)
		}
	}
}

// What rule 1 refuses besides anchors, aliases and merge keys is still
// read for now: a custom tag, a second document, and a quoted << that is
// a plain key; and a file of comments alone is an empty config.
func TestOtherYAMLIsStillRead(t *testing.T) {
	for _, text := range []string{
		"exclude: [a]\nx: !thing 1\n",
		"exclude: [a]\n---\nx: 1\n",
		"exclude: [a]\n\"<<\": {x: 1}\n",
	} {
		c, err := Parse([]byte(text))
		if err != nil || !slices.Equal(c.Selection.Exclude, []string{"a"}) {
			t.Errorf("%q: config %+v, error %v, want exclude [a]", text, c, err)
		}
	}
	if c, err := Parse([]byte("# nothing yet\n")); err != nil || len(c.Selection.Exclude) != 0 || c.Tests != nil {
		t.Errorf("comments alone: config %+v, error %v, want an empty config", c, err)
	}
}
