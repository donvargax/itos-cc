package exceptions_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

var entry = mutants.Exception{File: "src/a.go", Function: "m/src#f", Hash: "0123456789abcdef", LineInFunction: 2, Column: 9,
	Original: "<", Replacement: "!=", Reason: "the loop only counts up"}

// The writer reads a missing itos-cc.yaml as no text, so entries added to
// none are a new document that Parse reads back, in order, a deletion's
// replacement empty.
func TestExceptDocumentAddsEntriesThatParseReads(t *testing.T) {
	deletion := entry
	deletion.Column, deletion.Original, deletion.Replacement = 3, "!", ""
	var data []byte
	for _, e := range []mutants.Exception{entry, deletion} {
		updated, replaced, err := exceptions.ExceptDocument(data, e)
		if err != nil || replaced {
			t.Fatalf("ExceptDocument(%+v): replaced %v, error %v, want a new entry", e, replaced, err)
		}
		data = updated
	}
	c, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Exceptions, []mutants.Exception{entry, deletion}) {
		t.Errorf("exceptions %+v, want both, in order, the deletion's replacement empty", c.Exceptions)
	}
}

func TestExceptDocumentReplacesTheSameSiteAndKeepsTheRest(t *testing.T) {
	other := entry
	other.Column = 20
	text := "# Settings.\nother: [1, 2] # kept\nmutation:\n  # Reviewed.\n  exceptions:\n" +
		"    # The first.\n    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: old}\n" +
		"    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 20, original: '<', replacement: '!=', reason: the loop only counts up}\n"
	data, replaced, err := exceptions.ExceptDocument([]byte(text), entry)
	if err != nil || !replaced {
		t.Fatalf("replaced %v, error %v, want the first entry replaced", replaced, err)
	}
	c, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Exceptions, []mutants.Exception{entry, other}) {
		t.Errorf("exceptions %+v, want the first reworded and the second as it was", c.Exceptions)
	}
	for _, kept := range []string{"# Settings.", "# kept", "# Reviewed.", "# The first.", "other:"} {
		if !strings.Contains(string(data), kept) {
			t.Errorf("itos-cc.yaml lost %q:\n%s", kept, data)
		}
	}
}

func TestRenewDocumentRewritesOnlyItsEntriesAndKeepsTheRest(t *testing.T) {
	// The first entry predates line_text, and reads with none.
	text := "# Settings.\nother: [1, 2] # kept\nmutation:\n  exceptions:\n" +
		"    # The first.\n    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: old}\n" +
		"    - {file: src/a.go, function: m/src#g, hash: 0123456789abcdef, line_in_function: 2, column: 20, original: '<', replacement: '!=', reason: kept, line_text: 'if a < b {'}\n"
	before, err := config.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if before.Exceptions[0].LineText != "" || before.Exceptions[1].LineText != "if a < b {" {
		t.Fatalf("line texts %q and %q, want none and the second's", before.Exceptions[0].LineText, before.Exceptions[1].LineText)
	}
	renewed := before.Exceptions[0]
	renewed.Hash, renewed.LineInFunction, renewed.LineText = "fedcba9876543210", 3, "for i < n {"
	data, err := exceptions.RenewDocument([]byte(text), []mutants.Renewal{{Old: before.Exceptions[0], New: renewed}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(after.Exceptions, []mutants.Exception{renewed, before.Exceptions[1]}) {
		t.Errorf("exceptions %+v, want the first renewed and the second as it was", after.Exceptions)
	}
	for _, kept := range []string{"# Settings.", "# kept", "# The first.", "other:"} {
		if !strings.Contains(string(data), kept) {
			t.Errorf("itos-cc.yaml lost %q:\n%s", kept, data)
		}
	}
	if _, err := exceptions.RenewDocument(data, []mutants.Renewal{{Old: before.Exceptions[0], New: renewed}}); err == nil {
		t.Error("renewing an entry that is no longer there: no error")
	}
}

func TestExceptDocumentFillsAnEmptyOrNullSetting(t *testing.T) {
	for _, text := range []string{"", "# Nothing yet.\n", "mutation:\n", "mutation:\n  exceptions:\n"} {
		t.Run(text, func(t *testing.T) {
			data, _, err := exceptions.ExceptDocument([]byte(text), entry)
			if err != nil {
				t.Fatal(err)
			}
			if c, err := config.Parse(data); err != nil || !slices.Equal(c.Exceptions, []mutants.Exception{entry}) {
				t.Errorf("exceptions %+v, error %v, want the entry", c, err)
			}
		})
	}
}

// A document Parse refuses is refused with its *InvalidError and no bytes,
// so the writer, which writes only the bytes it is given, leaves the file
// as it was.
func TestExceptDocumentRefusesAnInvalidDocument(t *testing.T) {
	for _, text := range []string{
		"mutation: [\n",
		"- a\n",
		"mutation: 5\n",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', replacement: '<='}\n",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 0, column: 1, original: '<', replacement: '<=', reason: ' '}\n",
		"mutation:\n  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', reason: r}\n",
	} {
		data, _, err := exceptions.ExceptDocument([]byte(text), entry)
		var invalid *config.InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%q: ExceptDocument: %v, want it to refuse with an InvalidError", text, err)
		}
		if data != nil {
			t.Errorf("%q: ExceptDocument gave %q, want no bytes", text, data)
		}
	}
}

// slice-20: the writer changes only the lines of the entry it adds or
// replaces; every other byte of the document comes back as it was.
func TestExceptDocumentChangesOnlyTheLinesOfItsEntry(t *testing.T) {
	other := "{file: src/b.go, function: m/src#g, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: other}"
	old := "{file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: old}"
	block := func(pad string) string {
		return pad + "- file: src/a.go\n" + pad + "  function: m/src#f\n" + pad + "  hash: 0123456789abcdef\n" + pad + "  line_in_function: 2\n" +
			pad + "  column: 9\n" + pad + "  original: <\n" + pad + "  replacement: '!='\n" + pad + "  reason: the loop only counts up\n"
	}
	for _, c := range []struct{ name, text, want string }{
		{"empty", "", "mutation:\n  exceptions:\n" + block("    ")},
		{"a comment alone", "# Nothing yet.\n", "# Nothing yet.\nmutation:\n  exceptions:\n" + block("    ")},
		{"no line break at the end", "x: 1", "x: 1\nmutation:\n  exceptions:\n" + block("    ")},
		{"a null mutation", "---\nmutation: ~  # none yet\n\nnext: 1\n", "---\nmutation:  # none yet\n  exceptions:\n" + block("    ") + "\nnext: 1\n"},
		{"a null list", "mutation:\n    exceptions:\n    tests: {list: l, run: 'r {pattern}', ids_pattern: '{ids}', join: {each: '{id}', sep: ' '}}\n",
			"mutation:\n    exceptions:\n" + block("      ") + "    tests: {list: l, run: 'r {pattern}', ids_pattern: '{ids}', join: {each: '{id}', sep: ' '}}\n"},
		{"no list", "mutation:\n  tests: {list: l, run: 'r {pattern}', ids_pattern: '{ids}', join: {each: '{id}',   sep: ' '}}\n\n# the end\n",
			"mutation:\n  tests: {list: l, run: 'r {pattern}', ids_pattern: '{ids}', join: {each: '{id}',   sep: ' '}}\n  exceptions:\n" + block("    ") + "\n# the end\n"},
		{"after the last entry", "mutation:\n  exceptions:\n  - " + other + "\n\n  # trailing\nexclude:   [a/**]\n",
			"mutation:\n  exceptions:\n  - " + other + "\n" + block("  ") + "\n  # trailing\nexclude:   [a/**]\n"},
		{"a replaced flow entry", "mutation:\n  exceptions:\n    # first\n    - " + old + "   # its line\n\n    - " + other + "\n",
			"mutation:\n  exceptions:\n    # first\n" + strings.Replace(block("    "), "src/a.go\n", "src/a.go   # its line\n", 1) + "\n    - " + other + "\n"},
		{"a flow list", "mutation:\n  exceptions: [" + other + ",\n     ]  # rewritten\n\nnext: 1\n", "mutation:\n  exceptions:\n    - file: src/b.go\n      function: m/src#g\n" +
			"      hash: 0123456789abcdef\n      line_in_function: 2\n      column: 9\n      original: <\n      replacement: '!='\n      reason: other\n" + block("    ") + "\nnext: 1\n"},
		{"line breaks of Windows", "a: 1\r\nmutation:\r\n  exceptions:\r\n  - " + other + "\r\n", "a: 1\r\nmutation:\r\n  exceptions:\r\n  - " + other + "\r\n" + strings.ReplaceAll(block("  "), "\n", "\r\n")},
	} {
		t.Run(c.name, func(t *testing.T) {
			data, _, err := exceptions.ExceptDocument([]byte(c.text), entry)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != c.want {
				t.Errorf("itos-cc.yaml\n%s\nwant\n%s", data, c.want)
			}
		})
	}
}

// A replaced entry whose reason is a block scalar is replaced whole, and
// the comment-like lines of its reason go with it.
func TestExceptDocumentReplacesABlockScalarWhole(t *testing.T) {
	text := "mutation:\n  exceptions:\n  - file: src/a.go\n    function: m/src#f\n    hash: 0123456789abcdef\n    line_in_function: 2\n    column: 9\n" +
		"    original: '<'\n    replacement: '!='\n    reason: |+\n      old\n      # still the reason\n\n  # above nothing\n"
	data, replaced, err := exceptions.ExceptDocument([]byte(text), entry)
	if err != nil || !replaced {
		t.Fatalf("replaced %v, error %v", replaced, err)
	}
	want := "mutation:\n  exceptions:\n  - file: src/a.go\n    function: m/src#f\n    hash: 0123456789abcdef\n    line_in_function: 2\n    column: 9\n" +
		"    original: <\n    replacement: '!='\n    reason: the loop only counts up\n  # above nothing\n"
	if string(data) != want {
		t.Errorf("itos-cc.yaml\n%s\nwant\n%s", data, want)
	}
}

// A layout the writer does not splice, a flow-style mutation mapping, is
// written by re-encoding the document, which still reads as the edit.
func TestExceptDocumentReencodesAFlowMapping(t *testing.T) {
	data, _, err := exceptions.ExceptDocument([]byte("mutation: {}\n"), entry)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := config.Parse(data); err != nil || !slices.Equal(c.Exceptions, []mutants.Exception{entry}) {
		t.Errorf("exceptions %+v, error %v, want the entry", c, err)
	}
}

func TestRenewDocumentChangesOnlyTheLinesOfItsEntries(t *testing.T) {
	text := "---\n# Settings.\n\nother: { each: \"{id}\", sep: \"|\" }\nlimit: 30  # kept\n\nmutation:\n  exceptions:\n" +
		"    - {file: src/a.go, function: m/src#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: '<', replacement: '!=', reason: kept}\n\n" +
		"    - file: src/a.go\n      function: m/src#g\n      hash: 0123456789abcdef\n      line_in_function: 2\n      column: 20\n      original: '<'\n      replacement: '!='\n      reason: >-\n        folded\n        reason\n"
	before, err := config.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	renewed := before.Exceptions[1]
	renewed.Hash, renewed.LineText = "fedcba9876543210", "for i < n {"
	data, err := exceptions.RenewDocument([]byte(text), []mutants.Renewal{{Old: before.Exceptions[1], New: renewed}})
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(text, "    - file: src/a.go\n      function: m/src#g")
	want := head + "    - file: src/a.go\n      function: m/src#g\n      hash: fedcba9876543210\n      line_in_function: 2\n      column: 20\n      original: <\n      replacement: '!='\n" +
		"      reason: folded reason\n      line_text: for i < n {\n"
	if string(data) != want {
		t.Errorf("itos-cc.yaml\n%s\nwant\n%s", data, want)
	}
}
