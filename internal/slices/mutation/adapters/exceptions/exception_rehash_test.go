package exceptions_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/exceptions"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// rehashed is text with entry i of its mutation.exceptions given hash
// through RehashDocument, the renewal's New differing in its reason too,
// which it never writes.
func rehashed(t *testing.T, text string, i int, hash string) string {
	t.Helper()
	c, err := settings.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	next := c.Exceptions[i]
	next.Hash, next.Reason = hash, "not written"
	data, err := exceptions.RehashDocument([]byte(text), []mutants.Renewal{{Old: c.Exceptions[i], New: next}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := settings.Parse(data)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	want := c.Exceptions[i]
	want.Hash = hash
	if after.Exceptions[i] != want {
		t.Errorf("entry %+v, want %+v", after.Exceptions[i], want)
	}
	return string(data)
}

// slice-68: --rehash changes only the text of an entry's hash, in the
// style it is written in, keeping the entry's comments, its other keys'
// layout and every other byte.
func TestRehashDocumentChangesOnlyTheHashText(t *testing.T) {
	entry := func(hash string) string {
		return "  - file: a.go\n    function: m#f\n    hash: " + hash + "\n    # the place\n    line_in_function: 2\n    column: 9\n" +
			"    original: <\n    replacement: <=\n    reason: kept\n    x-ticket: QA-12\n"
	}
	for _, c := range []struct{ name, was, now, hash string }{
		{"plain", "0123456789abcdef", "fedcba9876543210", "fedcba9876543210"},
		{"single-quoted", "'0123456789abcdef'", "'fedcba9876543210'", "fedcba9876543210"},
		{"double-quoted", `"0123456789abcdef"`, `"fedcba9876543210"`, "fedcba9876543210"},
		{"plain, now a number", "0123456789abcdef", "'1234567890123456'", "1234567890123456"},
		{"single-quoted, a quote inside", "'0123''456789abcdef'", "'fedcba9876543210'", "fedcba9876543210"},
	} {
		text := "version: 1\nmutation:\n  exceptions:\n" + entry("0000000000000000") + entry(c.was)
		if got, want := rehashed(t, text, 1, c.hash), strings.Replace(text, "hash: "+c.was, "hash: "+c.now, 1); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, want)
		}
	}
	// A plain hash that needs quotes takes the document's double quotes.
	text := "exclude: [\"a\", \"b\"]\nmutation:\n  exceptions:\n" + entry("0123456789abcdef")
	if got, want := rehashed(t, text, 0, "1234567890123456"), strings.Replace(text, "0123456789abcdef", `"1234567890123456"`, 1); got != want {
		t.Errorf("double-quoted document:\n%s\nwant:\n%s", got, want)
	}
	// A flow-style list keeps its style.
	text = "mutation:\n  exceptions: [{file: a.go, function: m#f, hash: 0123456789abcdef, line_in_function: 2, column: 9, original: <, replacement: <=, reason: kept}]\n"
	if got, want := rehashed(t, text, 0, "fedcba9876543210"), strings.Replace(text, "0123456789abcdef", "fedcba9876543210", 1); got != want {
		t.Errorf("flow:\n%s\nwant:\n%s", got, want)
	}
}

// A hash RehashDocument cannot change in place, tagged, escaped or given
// a value no hash has, is written with its entry, as RenewDocument writes
// one; no renewal leaves the bytes as they were, and an entry that is not
// there is refused.
func TestRehashDocumentRewritesTheEntryOfAHashItCannotSplice(t *testing.T) {
	for _, c := range []struct{ name, hash, now string }{
		{"tagged", "!!str 0123456789abcdef", "fedcba9876543210"},
		{"escaped", `"\x30123456789abcdef"`, "fedcba9876543210"},
		{"literal", "|-\n      0123456789abcdef", "fedcba9876543210"},
		{"a quote in a single-quoted hash", "'0123456789abcdef'", "fedc'ba"},
	} {
		text := "mutation:\n  exceptions:\n  - file: a.go\n    function: m#f\n    hash: " + c.hash + "\n    line_in_function: 2\n    column: 9\n    original: <\n    replacement: <=\n    reason: kept\n"
		got := rehashed(t, text, 0, c.now)
		if strings.Contains(got, c.hash) {
			t.Errorf("%s: the hash's text stayed:\n%s", c.name, got)
		}
	}
	text := "# kept\nmutation:\n  exceptions:\n  - {file: a.go, function: m#f, hash: '01', line_in_function: 2, column: 9, original: <, replacement: <=, reason: kept}\n"
	data, err := exceptions.RehashDocument([]byte(text), nil)
	if err != nil || string(data) != text {
		t.Errorf("no renewal: %q, %v", data, err)
	}
	if _, err := exceptions.RehashDocument([]byte(text), []mutants.Renewal{{Old: mutants.Exception{File: "b.go"}}}); err == nil {
		t.Error("rehashing an entry that is not there: no error")
	}
	if _, err := exceptions.RehashDocument([]byte("mutation: [\n"), nil); err == nil {
		t.Error("a document that cannot be read: no error")
	}
}
