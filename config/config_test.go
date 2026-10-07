package config

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

var entry = Exception{File: "src/a.go", Function: "m/src#f", Hash: "0123456789abcdef", LineInFunction: 2, Column: 9,
	Original: "<", Replacement: "!=", Reason: "the loop only counts up"}

func TestAMissingFileIsAnEmptyConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	c, err := Load()
	if err != nil || len(c.Exceptions) != 0 {
		t.Errorf("config %+v, error %v, want none of either", c, err)
	}
}

func TestExceptWritesAnEntryThatLoadReads(t *testing.T) {
	t.Chdir(t.TempDir())
	deletion := entry
	deletion.Column, deletion.Original, deletion.Replacement = 3, "!", ""
	for _, e := range []Exception{entry, deletion} {
		if replaced, err := Except(e); err != nil || replaced {
			t.Fatalf("Except(%+v): replaced %v, error %v, want a new entry", e, replaced, err)
		}
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Exceptions, []Exception{entry, deletion}) {
		t.Errorf("exceptions %+v, want both, in order, the deletion's replacement empty", c.Exceptions)
	}
}

func TestExceptReplacesTheSameSiteAndKeepsTheRest(t *testing.T) {
	t.Chdir(t.TempDir())
	other := entry
	other.Column = 20
	text := "# Settings.\nother: [1, 2] # kept\nmutation:\n  # Reviewed.\n  exceptions:\n" +
		"    # The first.\n    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: old}\n" +
		"    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 20, original: '<', replacement: '!=', reason: the loop only counts up}\n"
	if err := os.WriteFile(File, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if replaced, err := Except(entry); err != nil || !replaced {
		t.Fatalf("replaced %v, error %v, want the first entry replaced", replaced, err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Exceptions, []Exception{entry, other}) {
		t.Errorf("exceptions %+v, want the first reworded and the second as it was", c.Exceptions)
	}
	data, _ := os.ReadFile(File)
	for _, kept := range []string{"# Settings.", "# kept", "# Reviewed.", "# The first.", "other:"} {
		if !strings.Contains(string(data), kept) {
			t.Errorf("itos-cc.yaml lost %q:\n%s", kept, data)
		}
	}
}

func TestExceptFillsAnEmptyOrNullSetting(t *testing.T) {
	for _, text := range []string{"", "# Nothing yet.\n", "mutation:\n", "mutation:\n  exceptions:\n"} {
		t.Run(text, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile(File, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Except(entry); err != nil {
				t.Fatal(err)
			}
			if c, err := Load(); err != nil || !slices.Equal(c.Exceptions, []Exception{entry}) {
				t.Errorf("exceptions %+v, error %v, want the entry", c, err)
			}
		})
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
		if _, err := Except(entry); !errors.As(err, &invalid) {
			t.Errorf("%q: Except: %v, want it to refuse with an InvalidError", text, err)
		}
		if data, _ := os.ReadFile(File); string(data) != text {
			t.Errorf("%q: Except wrote %q", text, data)
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
	c, err := parse([]byte(testsYAML))
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
	c, err = parse([]byte(testsYAML + "    whole: go test ./e2e\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Tests.All([]string{"A"}); got != "go test ./e2e" {
		t.Errorf("All(A) = %q, want whole", got)
	}
	c, err = parse([]byte(testsYAML + "    support: [\"features/*_test.go\"]\n"))
	if err != nil || !slices.Equal(c.Tests.Support, []string{"features/*_test.go"}) {
		t.Errorf("support %v, %v, want the glob", c, err)
	}
	if c, err := parse([]byte("mutation: {}\n")); err != nil || c.Tests != nil {
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
		_, err := parse([]byte(strings.Replace(testsYAML, c.from, c.to, 1)))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("without %q: error %v, want an *InvalidError that says %q", c.from, err, c.says)
		}
	}
}
