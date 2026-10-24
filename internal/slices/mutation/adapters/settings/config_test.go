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
