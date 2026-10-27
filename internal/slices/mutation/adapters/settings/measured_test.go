package settings

import (
	"errors"
	"testing"
)

// Measured is held by the Fingerprints contract through testinputs, and by
// ID-MUT-393 and ID-MUT-394 through the binary; these cases hold it in this
// package, whose own tests are the ones its coverage is measured by.

const measuredTests = "mutation:\n  tests:\n    list: list\n    run: run {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n"

func measuredOf(t *testing.T, text string) string {
	t.Helper()
	got, err := Measured([]byte(text))
	if err != nil {
		t.Fatalf("Measured(%q): %v", text, err)
	}
	return got
}

func TestMeasuredLeavesOutExceptionsExtensionKeysAndLayout(t *testing.T) {
	exception := "  exceptions:\n    - {file: a.go, function: f, hash: h, line_in_function: 1, column: 1, original: '<', replacement: '<=', reason: r, x-seen: today}\n"
	none := measuredOf(t, "")
	for _, text := range []string{"# a comment alone\n", "~\n", "version: 1\n", "version: 1\nmutation:\n" + exception, "x-owner: {team: a}\n"} {
		if got := measuredOf(t, text); got != none {
			t.Errorf("%q measured %q, not as no file, %q", text, got, none)
		}
	}
	tests := measuredOf(t, measuredTests)
	for _, text := range []string{
		"x-owner: team\n# the tests\n" + measuredTests + exception,
		"version: 1\nmutation:\n" + exception + "  x-note: n\n" + measuredTests[len("mutation:\n"):],
		"mutation: {tests: {join: {sep: ',', each: '{id}'}, ids_pattern: '{ids}', run: 'run {pattern}', list: list}}\n",
	} {
		if got := measuredOf(t, text); got != tests {
			t.Errorf("%q measured %q, not as its tests alone, %q", text, got, tests)
		}
	}
	if tests == none {
		t.Error("mutation.tests counts nothing")
	}
	for _, text := range []string{"exclude: [a.go]\n" + measuredTests, measuredTests + "    whole: all\n", measuredTests + "  copy: [dist]\n", measuredTests + "    support: [s.txt]\n"} {
		if got := measuredOf(t, text); got == tests {
			t.Errorf("%q measured as its tests alone", text)
		}
	}
}

func TestMeasuredRefusesWhatParseRefuses(t *testing.T) {
	for _, text := range []string{"mutation: [\n", "version: 2\n", "x-a: &a 1\n", "mutation:\n  exceptions:\n    - {file: a.go}\n"} {
		var invalid *InvalidError
		if got, err := Measured([]byte(text)); !errors.As(err, &invalid) || got != "" {
			t.Errorf("Measured(%q) = %q, %v, want an *InvalidError", text, got, err)
		}
	}
}

func TestDeclaredIsTheVersionAFileReadsAs(t *testing.T) {
	for text, want := range map[string]int{"": 1, "~\n": 1, "exclude: [a.go]\n": 1, "exclude: [a.go]\nversion: 1\n": 1} {
		_, top, err := parse([]byte(text))
		if got := declared(top); err != nil || got != want {
			t.Errorf("declared(%q) = %d, %v, want %d", text, got, err, want)
		}
	}
}
