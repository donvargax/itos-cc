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
