package features

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// -skip-scenarios leaves out what -scenarios selects by the same
// expression, as go test's -skip mirrors -run, so cd.yml's two features
// parts, one given the expression by each flag, run every live scenario
// once between them: a scenario with a tag the expression matches in the
// first alone, and every other in the second alone (T-223). The tags it
// matches are any a scenario carries, its feature's, its own and its
// Examples block's, as -scenarios reads them.
func TestSkipScenariosRunsWhatScenariosLeaves(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "")
	dir := partsFixture(t)
	every := ranByParts(t, dir, "", "")
	if want := []string{"skipped", "zero", "one", "five", "fifteen"}; !slices.Equal(every, want) {
		t.Fatalf("every live scenario: %v; want %v", every, want)
	}
	for _, pattern := range []string{
		"^@ID-[A-Z]+-[0-9]*[0-4]$", // cd.yml's: ID numbers ending 0 to 4
		"^@ID-UNIT-9[25]$",         // a scenario outline and a @wip scenario
		"^@T-90$",                  // an Examples block's tag
		"^@slow$",                  // a scenario's tag beside its ID
		"^@shared$",                // a feature's tag: every scenario
	} {
		t.Run(pattern, func(t *testing.T) {
			selected := ranByParts(t, dir, pattern, "")
			left := ranByParts(t, dir, "", pattern)
			for _, name := range every {
				in := slices.Contains(selected, name)
				if in == slices.Contains(left, name) {
					t.Errorf("%s: run by -scenarios %v, by -skip-scenarios %v; want exactly one", name, in, !in)
				}
			}
			if both := append(slices.Clone(selected), left...); len(both) != len(every) {
				t.Errorf("-scenarios ran %v and -skip-scenarios %v; want each of %v once", selected, left, every)
			}
		})
	}
}

// The selection fixture with units.feature holding the shapes a tag can
// select by: a feature's tag, a scenario's beside its ID, a @wip
// scenario's, an outline's and an Examples block's. Each scenario, and
// each row, runs a fixture of its own name.
func partsFixture(t *testing.T) string {
	t.Helper()
	dir := featureSelectionFixture(t)
	units := "@shared\nFeature: Parts\n" +
		"  @ID-UNIT-90\n  Scenario: Zero\n    Given the zero fixture runs\n\n" +
		"  @ID-UNIT-91 @slow\n  Scenario: One\n    Given the one fixture runs\n\n" +
		"  @wip @ID-UNIT-92\n  Scenario: Pending\n    Given the pending fixture runs\n\n" +
		"  @ID-UNIT-95\n  Scenario Outline: Rows\n    Given the <which> fixture runs\n\n" +
		"    Examples:\n      | which |\n      | five  |\n\n" +
		"    @T-90\n    Examples:\n      | which   |\n      | fifteen |\n"
	if err := os.WriteFile(filepath.Join(dir, "units.feature"), []byte(units), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ranByParts is what the binary driver runs of the feature files in dir
// given -scenarios=run and -skip-scenarios=skip, each flag set back after
// the test.
func ranByParts(t *testing.T, dir, run, skip string) []string {
	t.Helper()
	setFlag(t, "scenarios", run)
	setFlag(t, "skip-scenarios", skip)
	sel, err := binarySelection(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter == "" {
		return nil
	}
	return runSelection(t, sel.Filter, sel.Paths)
}

func setFlag(t *testing.T, name, value string) {
	t.Helper()
	f := flag.Lookup(name)
	if f == nil {
		t.Fatalf("the features tests take no -%s", name)
	}
	before := f.Value.String()
	if err := flag.Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.Set(name, before) })
}
