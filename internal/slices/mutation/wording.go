package mutation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Quote retains the existing mutation presentation vocabulary.
func Quote(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}

// LanguageName retains the existing mutation presentation vocabulary.
func LanguageName(language string) string {
	switch language {
	case "go":
		return "Go"
	case "typescript":
		return "TypeScript"
	}
	if language == "" {
		return language
	}
	return strings.ToUpper(language[:1]) + language[1:]
}

// CoverageInventory retains the existing mutation presentation vocabulary.
func CoverageInventory(language string) string {
	if language == "go" {
		return "Go coverage inventory"
	}
	return LanguageName(language) + " line coverage inventory"
}

// ExecutableUnit retains the existing mutation presentation vocabulary.
func ExecutableUnit(language string) string {
	if language == "go" {
		return "executable Go coverage block"
	}
	return "executable " + LanguageName(language) + " line"
}

// CoverageEvidenceName retains the existing mutation presentation vocabulary.
func CoverageEvidenceName(language string) string {
	if language == "go" {
		return "Go coverage"
	}
	return LanguageName(language) + " line coverage"
}

// ExceptedIn retains the existing mutation presentation vocabulary.
func ExceptedIn(items []mutants.Mutant) int {
	n := 0
	for _, m := range items {
		if m.Excepted != "" {
			n++
		}
	}
	return n
}

// StrictLanguages retains the existing mutation presentation vocabulary.
func StrictLanguages(sources []string) []string {
	var out []string
	for _, source := range sources {
		if spec := lang.Detect(source); spec != nil && mutants.StrictCoverage(spec.Name) && !slices.Contains(out, spec.Name) {
			out = append(out, spec.Name)
		}
	}
	slices.SortFunc(out, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "go":
			return -1
		case b == "go":
			return 1
		}
		return strings.Compare(a, b)
	})
	return out
}

func staleBecause(fn mutants.FunctionCheck) string {
	tests := fn.Tests
	switch {
	case fn.Marked:
		return "has mutation results from before the tests that import its file changed, kept by a mutation run that did not judge it"
	case len(fn.Listed) > 0:
		return "has kills by listed tests made before files they rest on changed: " + strings.Join(fn.Listed, ", ")
	case len(fn.Broad) > 0:
		return "has whole-suite results made before inputs changed: " + strings.Join(fn.Broad, ", ")
	case len(fn.Unrecorded) > 0:
		var sites []string
		for _, s := range fn.Unrecorded {
			sites = append(sites, fmt.Sprintf("%d:%d `%s` → `%s`", s.Line, s.Column, s.Original, s.Replacement))
		}
		return "has sites its mutation results never recorded, as when a newer itos-cc adds a mutation operator or a fail-fast stop left them undecided: " + strings.Join(sites, ", ")
	case tests == nil:
		return "changed since its mutation results"
	case tests.Unrecorded:
		return "has mutation results from before snapshots recorded the tests that import its file"
	}
	return "has mutation results from before the tests that import its file changed: " + testChanges(tests)
}

func testChanges(tests *mutants.TestChange) string {
	var changes []string
	for _, c := range []struct {
		what  string
		paths []string
	}{{"changed", tests.Changed}, {"added", tests.Added}, {"removed", tests.Removed}} {
		if len(c.paths) > 0 {
			changes = append(changes, c.what+" "+strings.Join(c.paths, ", "))
		}
	}
	return strings.Join(changes, "; ")
}

func testsNote(functions []mutants.FunctionCheck) string {
	for _, fn := range functions {
		switch {
		case fn.Tests == nil:
		case fn.Tests.Unrecorded:
			return " (its results predate snapshots recording tests)"
		default:
			return " (the tests that import it: " + testChanges(fn.Tests) + ")"
		}
	}
	return ""
}
