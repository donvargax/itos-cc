package exceptions_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// quoted is an entry three of whose strings need quotes: a replacement
// starting with "!", a reason holding ": " and a line text holding " #".
var quoted = mutants.Exception{File: "src/a.go", Function: "m/src#f", Hash: "0123456789abcdef", LineInFunction: 2, Column: 9,
	Original: "<", Replacement: "!=", Reason: "x is never 5: the caller checks", LineText: "x := n # one"}

// quotedLines is quoted as a block entry, its dash at column 5, the
// strings that need quotes in q.
func quotedLines(q string) string {
	return "    - file: src/a.go\n      function: m/src#f\n      hash: 0123456789abcdef\n      line_in_function: 2\n      column: 9\n" +
		"      original: <\n      replacement: " + q + "!=" + q + "\n      reason: " + q + "x is never 5: the caller checks" + q + "\n" +
		"      line_text: " + q + "x := n # one" + q + "\n"
}

// slice-66 (issue #64): a string the writer must quote takes double quotes
// when the document holds more double-quoted scalars than single-quoted
// ones, any key or value, x- keys' included, and single quotes otherwise;
// a string that needs none stays plain and a number a number.
func TestAnAddedEntryTakesTheQuoteTheFileUses(t *testing.T) {
	for _, c := range []struct{ text, quote string }{
		{"exclude: [\"**/gen/**\"]\n", `"`},
		{"exclude: ['**/gen/**']\n", `'`},
		{"exclude: [gen/**]\n", `'`},
		{"exclude: [\"a\", 'b']\n", `'`},
		{"x-note: \"theirs\"\nexclude: [\"a\", 'b']\n", `"`},
		{"\"exclude\": [a, 'b']\n", `'`},
		{"\"exclude\": [\"a\", 'b']\n", `"`},
	} {
		data, _, err := exceptions.ExceptDocument([]byte(c.text), quoted)
		if err != nil {
			t.Fatal(err)
		}
		if want := c.text + "mutation:\n  exceptions:\n" + quotedLines(c.quote); string(data) != want {
			t.Errorf("itos-cc.yaml %q given the entry\n%s\nwant\n%s", c.text, data, want)
		}
	}
}

// A renewed entry in a double-quoted file is written in double quotes, and
// nothing else changes: not the entries around it, nor its own x- key,
// single-quoted as its writer had it.
func TestARenewedEntryTakesTheFilesDoubleQuotes(t *testing.T) {
	other := "    - file: src/b.go\n      function: m/src#g\n      hash: 0123456789abcdef\n      line_in_function: 2\n      column: 9\n" +
		"      original: <\n      replacement: \"!=\"\n      reason: \"kept: as it was\"\n"
	head := "exclude: [\"**/gen/**\"]\nmutation:\n  exceptions:\n" + other
	last := strings.Replace(other, "m/src#g", "m/src#h", 1)
	text := head + strings.Replace(quotedLines(`"`), "  - file: src/a.go\n", "  - file: src/a.go\n      x-note: 'theirs'\n", 1) + last
	c, err := settings.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	renewed := c.Exceptions[1]
	renewed.Hash = "fedcba9876543210"
	data, err := exceptions.RenewDocument([]byte(text), []mutants.Renewal{{Old: c.Exceptions[1], New: renewed}})
	if err != nil {
		t.Fatal(err)
	}
	entry := strings.Replace(quotedLines(`"`), "0123456789abcdef", "fedcba9876543210", 1) + "      x-note: 'theirs'\n"
	if want := head + entry + last; string(data) != want {
		t.Errorf("itos-cc.yaml\n%s\nwant\n%s", data, want)
	}
}

// A document re-encoded whole, its mutation mapping in flow style, writes
// the entries it adds and replaces in the quote it uses.
func TestAReencodedEntryTakesTheFilesDoubleQuotes(t *testing.T) {
	for _, text := range []string{
		"mutation: {copy: [\"dist/**\"]}\n",
		"mutation: {copy: [\"dist/**\"], exceptions: [{file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: \"<\", replacement: \"!=\", reason: old}]}\n",
	} {
		data, _, err := exceptions.ExceptDocument([]byte(text), quoted)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{`replacement: "!="`, `reason: "x is never 5: the caller checks"`, `line_text: "x := n # one"`, "original: <"} {
			if !strings.Contains(string(data), line) {
				t.Errorf("itos-cc.yaml %q, re-encoded, does not hold %s:\n%s", text, line, data)
			}
		}
	}
}
