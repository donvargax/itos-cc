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
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: ' ', replacement: '', reason: r}\n":     "has no original",
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

// mutation.copy holds doublestar globs from the project root, in order,
// each checked as exclude's are and refused by its own key (slice-43).
func TestMutationCopyReadsAndChecksItsGlobs(t *testing.T) {
	c, err := Parse([]byte("mutation:\n  copy: [gen, \"dist/**/*.d.ts\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"gen", "dist/**/*.d.ts"}; !slices.Equal(c.Copy, want) {
		t.Errorf("mutation.copy %q, want %q", c.Copy, want)
	}
	_, err = Parse([]byte("mutation:\n  copy: [gen, \"../x/**\"]\n"))
	var invalid *InvalidError
	if !errors.As(err, &invalid) || invalid.Key != "mutation.copy[1]" || !strings.Contains(invalid.Reason, "mutation.copy[1] \"../x/**\" leaves the project root") {
		t.Errorf("error %v, want mutation.copy[1] refused for leaving the project root", err)
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

// A tag written in the file other than ! and JSON's is refused at the key
// whose value carries it, a key that is not a string at the mapping holding
// it, and a second document naming the file alone, each before anything
// else is read, with a fix of its own (docs/CONFIG.md rule 1, slice-49).
func TestTagsKeysThatAreNotStringsAndASecondDocumentAreRefused(t *testing.T) {
	for _, c := range []struct{ text, key, found, fix string }{
		{"exclude: [!foo \"**/porttest/**\"]\n", "exclude[0]", "exclude[0] carries the tag !foo, and itos-cc.yaml takes no tags but ! and JSON's", tagFix},
		{"{x: !!python/object:os.system echo, exclude: []}\n", "x", "x carries the tag !!python/object:os.system, and itos-cc.yaml takes no tags but ! and JSON's", tagFix},
		{"mutation: {exceptions: !foo []}\n", "mutation.exceptions", "mutation.exceptions carries the tag !foo", tagFix},
		{"x: !!timestamp 2026-10-24\n", "x", "x carries the tag !!timestamp", tagFix},
		{"!foo {exclude: []}\n", "", "the top level carries the tag !foo", tagFix},
		{"mutation: {[a]: b}\n", "mutation", "mutation holds a key that is not a string, and itos-cc.yaml takes string keys alone", keyFix},
		{"{a: b}: c\n", "", "the top level holds a key that is not a string", keyFix},
		{"exclude: [\"**/porttest/**\"]\n---\nexclude: []\n", "", "a second document follows ---, and itos-cc.yaml holds one document alone", documentFix},
		{"exclude: [!foo a]\n---\nx: [\n", "exclude[0]", "exclude[0] carries the tag !foo", tagFix},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%q: error %v, want an *InvalidError", c.text, err)
			continue
		}
		if invalid.Key != c.key || !strings.HasPrefix(err.Error(), "itos-cc.yaml: "+c.found) || invalid.Fix != c.fix {
			t.Errorf("%q: key %q, error %q, fix %q; want %q, %q and %q", c.text, invalid.Key, err, invalid.Fix, c.key, c.found, c.fix)
		}
	}
}

// The non-specific tag, JSON's tags, a leading --- and a plain value YAML
// resolves as a timestamp are read as plain data, and a file of comments
// alone, or of a lone ---, is an empty config. A quoted << is a plain key,
// refused as one the format does not list (keys_test.go).
func TestJSONsYAMLIsRead(t *testing.T) {
	for _, text := range []string{
		"exclude: [! a]\n",
		"exclude: !!seq [!!str a]\nmutation: !!map {tests: !!null null, exceptions: [{file: a.go, function: f, hash: h, line_in_function: !!int 1, column: !!int 1, original: '<', replacement: '<=', reason: !!bool true, line_text: !!float 1.5}]}\n",
		"exclude: !<tag:yaml.org,2002:seq> [a]\n",
		"--- {exclude: [a]}\n",
		"exclude: [a]\n...\n",
	} {
		c, err := Parse([]byte(text))
		if err != nil || !slices.Equal(c.Selection.Exclude, []string{"a"}) {
			t.Errorf("%q: config %+v, error %v, want exclude [a]", text, c, err)
		}
	}
	c, err := Parse([]byte("mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', replacement: '<=', reason: 2026-10-24}\n"))
	if err != nil || len(c.Exceptions) != 1 || c.Exceptions[0].Reason != "2026-10-24" {
		t.Errorf("a plain timestamp: config %+v, error %v, want the reason 2026-10-24", c, err)
	}
	for _, text := range []string{"", "# nothing yet\n", "---\n"} {
		if c, err := Parse([]byte(text)); err != nil || len(c.Selection.Exclude) != 0 || c.Tests != nil {
			t.Errorf("%q: config %+v, error %v, want an empty config", text, c, err)
		}
	}
}

// version, a positive integer as JSON writes one, is read up to Version; a
// file without one reads as version 1, and one whose top level is not a
// mapping is left to decoding. The scenarios hold one newer version and
// three values that are not positive integers; these hold the rest of what
// is refused, how each is named, and the order against the other checks
// (docs/CONFIG.md rules 5 and 6, slice-53).
func TestVersionIsAPositiveIntegerUpToTheOneThisItosCCReads(t *testing.T) {
	for _, text := range []string{
		"version: 1\n", "exclude: []\nversion: 1\n", "version: !!int 1\n", "{exclude: []}\n", "# no version\n",
	} {
		if _, err := Parse([]byte(text)); err != nil {
			t.Errorf("%q: error %v, want none", text, err)
		}
	}
	const notWhole = "Write version: 1, the version this itos-cc writes, or remove the key, which reads as version 1."
	for _, c := range []struct{ text, found, fix string }{
		{"exclude: []\nversion: 2\n", "itos-cc.yaml: version is 2, newer than 1, the newest this itos-cc reads",
			"The file was written for a newer itos-cc: upgrade itos-cc to a release that reads version 2."},
		{"version: \"1\"\n", `version is "1", and itos-cc.yaml's version is a whole number, 1 or more, written without quotes`, notWhole},
		{"version: '1'\n", `version is "1"`, notWhole},
		{"version: 1.5\n", "version is 1.5,", notWhole},
		{"version: 0\n", "version is 0,", notWhole},
		{"version: -1\n", "version is -1,", notWhole},
		{"version: 01\n", "version is 01,", notWhole},
		{"version: 0x1\n", "version is 0x1,", notWhole},
		{"version: 99999999999999999999\n", "version is 99999999999999999999,", notWhole},
		{"version: !!str 1\n", "version is 1,", notWhole},
		{"version:\n", "version is empty,", notWhole},
		{"version: [1]\n", "version is a list,", notWhole},
		{"version: {v: 1}\n", "version is a mapping,", notWhole},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || invalid.Key != "version" || !strings.Contains(err.Error(), c.found) || invalid.Fix != c.fix {
			t.Errorf("%q: error %v, want an *InvalidError keyed version saying %q, its fix %q", c.text, err, c.found, c.fix)
		}
	}
	// After rule 1's refusals, before a key the file does not list.
	for _, c := range []struct{ text, key, found string }{
		{"version: &v 2\n", "version", "carries the anchor &v"},
		{"version: !foo 2\n", "version", "carries the tag !foo"},
		{"{colour: red, version: 2}\n", "version", "newer than 1"},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || invalid.Key != c.key || !strings.Contains(err.Error(), c.found) {
			t.Errorf("%q: error %v, want an *InvalidError keyed %q saying %q", c.text, err, c.key, c.found)
		}
	}
}
