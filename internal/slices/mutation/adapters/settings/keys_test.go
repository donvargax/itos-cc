package settings

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A key the format does not list is refused at its path from the top, the
// first in document order, after rule 1's refusals and before any value is
// checked; its fix names the listed key at that place within two edits,
// the first of the nearest in the table's order, or else every key listed
// there (docs/CONFIG.md rule 2, slice-52). The scenarios hold one row per
// level; these hold the ties, the orders and the places the walk leaves to
// decoding, which a scenario would only repeat.
func TestAKeyTheFormatDoesNotListIsRefused(t *testing.T) {
	const listsTop = "Remove the key, or rename it to one the top level takes: exclude, mutation."
	for _, c := range []struct{ text, key, holder, name, fix string }{
		{"excludes: [a]\n", "excludes", "the top level", "excludes", "Did you mean exclude? Rename the key, or remove it."},
		{"{colour: red}\n", "colour", "the top level", "colour", listsTop},
		{"{zz: 1, exclude: [], aa: 2}\n", "zz", "the top level", "zz", listsTop},
		{"{exclude: 5, colour: red}\n", "colour", "the top level", "colour", listsTop},
		{"exclude: [a]\n\"<<\": {x: 1}\n", "<<", "the top level", "<<", listsTop},
		{"mutation: {exceptons: []}\n", "mutation.exceptons", "mutation", "exceptons", "Did you mean exceptions? Rename the key, or remove it."},
		{"mutation:\n  exceptions:\n    - {file: a.go, line_txt: x}\n", "mutation.exceptions[0].line_txt", "mutation.exceptions[0]", "line_txt", "Did you mean line_text? Rename the key, or remove it."},
		{"mutation: {exceptions: [{}, {colour: x}]}\n", "mutation.exceptions[1].colour", "mutation.exceptions[1]", "colour",
			"Remove the key, or rename it to one mutation.exceptions[1] takes: file, function, hash, line_in_function, column, original, replacement, reason, line_text."},
		{"mutation: {tests: {join: {eap: x}}}\n", "mutation.tests.join.eap", "mutation.tests.join", "eap", "Did you mean each? Rename the key, or remove it."},
		{"mutation: {tests: {wholly: x}}\n", "mutation.tests.wholly", "mutation.tests", "wholly", "Did you mean whole? Rename the key, or remove it."},
		{"mutation: {cpy: [a]}\n", "mutation.cpy", "mutation", "cpy", "Did you mean copy? Rename the key, or remove it."},
		{"mutation: {copyxyz: [a]}\n", "mutation.copyxyz", "mutation", "copyxyz", "Remove the key, or rename it to one mutation takes: exceptions, tests, copy."},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%q: error %v, want an *InvalidError", c.text, err)
			continue
		}
		want := "itos-cc.yaml: " + c.holder + " holds the key " + c.name + ", which itos-cc.yaml does not list"
		if invalid.Key != c.key || err.Error() != want || invalid.Fix != c.fix {
			t.Errorf("%q: key %q, error %q, fix %q; want %q, %q and %q", c.text, invalid.Key, err, invalid.Fix, c.key, want, c.fix)
		}
	}
}

// An extension key, starting with x-, is accepted and ignored at every
// level the format lists keys at, the deeper ones included, which the
// scenarios hold at the top level and mutation alone; its value is still
// held to rule 1, and a key that only looks like one is refused (slice-54).
func TestAnExtensionKeyIsIgnoredAtAnyLevel(t *testing.T) {
	for _, text := range []string{
		"{x-: 1, x-owner: team}\n",
		"mutation: {exceptions: [{file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', replacement: '>', reason: r, x-seen: [1, {a: b}]}]}\n",
		"mutation: {tests: {list: l, run: 'r {pattern}', ids_pattern: '{ids}', join: {each: '{id}', sep: ' ', x-why: none}, x-note: n}}\n",
	} {
		if _, err := Parse([]byte(text)); err != nil {
			t.Errorf("%q: error %v, want none", text, err)
		}
	}
	for _, c := range []struct{ text, key, found string }{
		{"{x-owner: &a team}\n", "x-owner", "x-owner carries the anchor &a"},
		{"mutation: {x-note: !foo n}\n", "mutation.x-note", "mutation.x-note carries the tag !foo"},
		{"mutation: {exceptions: [{X-seen: 1}]}\n", "mutation.exceptions[0].X-seen", "does not list"},
		{"mutation: {tests: {x_note: n}}\n", "mutation.tests.x_note", "does not list"},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || invalid.Key != c.key || !strings.Contains(err.Error(), c.found) {
			t.Errorf("%q: error %v, want an *InvalidError keyed %q saying %q", c.text, err, c.key, c.found)
		}
	}
}

// edits counts the letters inserted, deleted or replaced, whichever word
// is the longer.
func TestEditsIsTheLevenshteinDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"", "abc", 3}, {"abc", "", 3}, {"same", "same", 0},
		{"kitten", "sitting", 3}, {"sitting", "kitten", 3}, {"flaw", "lawn", 2}, {"eap", "sep", 2}, {"ab", "ba", 2},
	} {
		if got := edits(c.a, c.b); got != c.want {
			t.Errorf("edits(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// Rule 1's refusals come first, and a value of another shape than its
// key's, such as a mapping where a list belongs, is left to decoding,
// whose refusal is not one of a key the format does not list.
func TestUnlistedKeysComeAfterRuleOneAndBeforeDecoding(t *testing.T) {
	for _, c := range []struct{ text, key, found string }{
		{"{colour: &a red}\n", "colour", "colour carries the anchor &a"},
		{"colour: !foo x\n", "colour", "colour carries the tag !foo"},
		{"colour: x\n---\nexclude: []\n", "", "a second document follows ---"},
		{"exclude: {a: b}\n", "", "cannot unmarshal"},
		{"mutation: {exceptions: {file: a}}\n", "", "cannot unmarshal"},
		{"mutation: [colour]\n", "", "cannot unmarshal"},
	} {
		_, err := Parse([]byte(c.text))
		var invalid *InvalidError
		if !errors.As(err, &invalid) || invalid.Key != c.key || !strings.Contains(err.Error(), c.found) || strings.Contains(err.Error(), "does not list") {
			t.Errorf("%q: error %v, want an *InvalidError keyed %q saying %q", c.text, err, c.key, c.found)
		}
	}
}

// format and raw, the decoder, name the same keys at every level: a key
// the table lists that nothing decodes would pass unread, and one the
// decoder reads that the table does not list could never be written. No
// scenario can see a key both the walk and the decoder lack.
func TestFormatListsTheKeysTheDecoderReads(t *testing.T) {
	var compare func(at string, s shape, typ reflect.Type)
	compare = func(at string, s shape, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Struct {
				if s.entry == nil {
					t.Errorf("%s: the decoder reads a list of mappings, format lists no entry", at)
					return
				}
				s = *s.entry
			}
			typ = typ.Elem()
		}
		var decoded []string
		if typ.Kind() == reflect.Struct {
			for i := range typ.NumField() {
				f := typ.Field(i)
				name := f.Tag.Get("yaml")
				decoded = append(decoded, name)
				value, ok := s.value(name)
				if !ok {
					t.Errorf("%s: the decoder reads %s, which format does not list", at, name)
					continue
				}
				compare(at+"."+name, value, f.Type)
			}
		}
		for _, k := range s.keys {
			if !slices.Contains(decoded, k.name) {
				t.Errorf("%s: format lists %s, which the decoder does not read", at, k.name)
			}
		}
	}
	compare("the top level", format, reflect.TypeFor[raw]())
}
