package features

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// Selection belongs to the test runners, which the product's command line
// cannot exercise. Tiny feature files show which scenarios actually run.
func TestSkippedFeatureSelectionRunsNothing(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "dry.feature")
	dir := featureSelectionFixture(t)
	sel, err := TagFilter("^@ID-DRY-90$", dir, Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter != "" {
		t.Fatalf("a selection naming only a skipped feature would run scenarios: %s", sel.Filter)
	}
}

func TestMixedFeatureSelectionRunsOnlyActiveScenarios(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "dry.feature")
	dir := featureSelectionFixture(t)
	sel, err := TagFilter("^@ID-(DRY|UNIT)-90$", dir, Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	ran := runSelection(t, sel.Filter, selectionPaths(dir, Converted))
	if !slices.Equal(ran, []string{"active"}) {
		t.Fatalf("scenarios ran: %v; want only active", ran)
	}
}

func TestFeatureSelectionRunsBothFilesByDefault(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "")
	dir := featureSelectionFixture(t)
	sel, err := TagFilter("", dir, Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	ran := runSelection(t, sel.Filter, selectionPaths(dir, Converted))
	if !slices.Equal(ran, []string{"skipped", "active"}) {
		t.Fatalf("scenarios ran: %v; want both files, in Converted's order, dry.feature's before units.feature's", ran)
	}
}

func TestFeatureSelectionRejectsUnknownTagsDespiteSkippedFiles(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "dry.feature")
	_, err := TagFilter("^@ID-UNKNOWN-90$", featureSelectionFixture(t), Converted, InProcessOnly)
	if err == nil || !strings.Contains(err.Error(), "no scenario has a tag") {
		t.Fatalf("unknown selection: %v; want a no-match failure", err)
	}
}

func TestFeatureSelectionValidatesUnsteppedIDsDespiteSkippedFiles(t *testing.T) {
	pendingSelectionFixture(t, "@ID-MUT-99991")
	t.Setenv("ITOS_CC_SKIP_FEATURES", "mutate.feature")
	dir := featureSelectionFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "mutate.feature"), []byte("Feature: Missing IDs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := TagFilter("", dir, Converted, InProcessOnly)
	if err == nil || !strings.Contains(err.Error(), "features.Unstepped lists "+Unstepped[0]) {
		t.Fatalf("missing Unstepped ID: %v; want its validation failure", err)
	}
}

func TestFeatureSelectionUsesActivePathsForSharedTags(t *testing.T) {
	for _, driver := range []struct {
		name, skip string
		files      []string
	}{
		{"binary", InProcessOnly, Converted},
		{"in process", BinaryOnly, Converted},
	} {
		t.Run(driver.name, func(t *testing.T) {
			for _, pattern := range []string{"", "^@shared$", "^@ID-(DRY|UNIT)-90$"} {
				t.Run(pattern, func(t *testing.T) {
					t.Setenv("ITOS_CC_SKIP_FEATURES", "dry.feature")
					sel, err := TagFilter(pattern, featureSelectionFixture(t), driver.files, driver.skip)
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(sel.Skipped, []string{"dry.feature"}) {
						t.Fatalf("omissions to log: %v; want dry.feature", sel.Skipped)
					}
					ran := runSelection(t, sel.Filter, sel.Paths)
					if !slices.Equal(ran, []string{"active"}) {
						t.Fatalf("scenarios ran: %v; want only active", ran)
					}
				})
			}
		})
	}
}

func TestFeatureSelectionOmitsCommaSeparatedFiles(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", " dry.feature, units.feature, dry.feature, ")
	sel, err := TagFilter("^@shared$", featureSelectionFixture(t), Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter != "" || !slices.Equal(sel.Skipped, []string{"dry.feature", "units.feature"}) {
		t.Fatalf("skipped-only selection: filter %q, omissions %v", sel.Filter, sel.Skipped)
	}
}

func TestFeatureSelectionRunsNothingWhenAllDriverFilesAreSkipped(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", strings.Join(Converted, ","))
	sel, err := TagFilter("", featureSelectionFixture(t), Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter != "" || len(sel.Paths) != 0 {
		t.Fatalf("all files skipped: filter %q, paths %v", sel.Filter, sel.Paths)
	}
}

func TestFeatureSelectionStillRecognizesUnsteppedOnlySelections(t *testing.T) {
	pendingSelectionFixture(t, "@ID-MUT-99992")
	t.Setenv("ITOS_CC_SKIP_FEATURES", "dry.feature")
	pattern := "^" + regexp.QuoteMeta(Unstepped[0]) + "$"
	sel, err := TagFilter(pattern, featureSelectionFixture(t), Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter != "" || !slices.Equal(sel.Unstepped, []string{Unstepped[0]}) {
		t.Fatalf("Unstepped-only selection: filter %q, unstepped %v", sel.Filter, sel.Unstepped)
	}
}

func featureSelectionFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range Converted {
		text := "Feature: " + name + "\n"
		switch name {
		case "dry.feature":
			text = "@shared\nFeature: Skipped file\n  @ID-DRY-90\n  Scenario: Skipped\n    Given the skipped fixture runs\n"
		case "units.feature":
			text = "@shared\nFeature: Active file\n  @ID-UNIT-90\n  Scenario: Active\n    Given the active fixture runs\n"
		case "mutate.feature":
			for i, id := range Unstepped {
				text += fmt.Sprintf("  @wip %s\n  Scenario: Unstepped %d\n    Given the skipped fixture runs\n", id, i)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func selectionPaths(dir string, files []string) []string {
	var paths []string
	for _, name := range files {
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths
}

func runSelection(t *testing.T, filter string, paths []string) []string {
	t.Helper()
	var ran []string
	suite := godog.TestSuite{
		Name: "selection",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^the (skipped|active) fixture runs$`, func(name string) error {
				ran = append(ran, name)
				return nil
			})
		},
		Options: &godog.Options{Paths: paths, Tags: filter, Format: "progress", Strict: true, Output: io.Discard},
	}
	if code := suite.Run(); code != 0 {
		t.Fatalf("fixture scenarios exited %d", code)
	}
	return ran
}

// Pending IDs are fixture input, independent of how many real conversions remain.
func pendingSelectionFixture(t *testing.T, ids ...string) {
	t.Helper()
	before := Unstepped
	Unstepped = ids
	t.Cleanup(func() { Unstepped = before })
}
func TestFeatureSelectionWithEmptyOrWIPOnlyPending(t *testing.T) {
	for _, pending := range [][]string{nil, {"@ID-MUT-99993"}} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) {
			pendingSelectionFixture(t, pending...)
			t.Setenv("ITOS_CC_SKIP_FEATURES", "")
			dir := featureSelectionFixture(t)
			sel, err := TagFilter("^@ID-UNIT-90$", dir, Converted, InProcessOnly)
			if err != nil {
				t.Fatal(err)
			}
			if ran := runSelection(t, sel.Filter, sel.Paths); !slices.Equal(ran, []string{"active"}) {
				t.Fatalf("active scenario lost with pending%v: %v", pending, ran)
			}
			if _, err := TagFilter("^@ID-UNKNOWN-91$", dir, Converted, InProcessOnly); err == nil || !strings.Contains(err.Error(), "no scenario has a tag") {
				t.Fatalf("unknown ID admitted with pending%v: %v", pending, err)
			}
		})
	}
}

// Deferred scenarios leave the in-process selection, by default and when a
// selection names them, and are reported; the binary driver's keeps them.
func TestDeferredScenariosLeaveOnlyTheirDriversSelection(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "")
	dir := featureSelectionFixture(t)
	deferred := map[string]string{"@ID-DRY-90": "a reason naming its owner"}
	for _, c := range []struct {
		pattern   string
		ran, left []string
	}{
		{"", []string{"active"}, []string{"@ID-DRY-90"}},
		{"^@ID-(DRY|UNIT)-90$", []string{"active"}, []string{"@ID-DRY-90"}},
		{"^@ID-UNIT-90$", []string{"active"}, nil},
		{"^@ID-DRY-90$", nil, []string{"@ID-DRY-90"}},
	} {
		t.Run(c.pattern, func(t *testing.T) {
			sel, err := TagFilter(c.pattern, dir, Converted, InProcessOnly)
			if err != nil {
				t.Fatal(err)
			}
			kept := runSelection(t, sel.Filter, sel.Paths)
			sel, left, err := Defer(sel, c.pattern, dir, deferred)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(left, c.left) {
				t.Fatalf("deferred left out %v; want %v", left, c.left)
			}
			if ran := runSelection(t, sel.Filter, sel.Paths); !slices.Equal(ran, c.ran) {
				t.Fatalf("scenarios ran: %v; want %v (without deferral %v)", ran, c.ran, kept)
			}
		})
	}
	if _, _, err := Defer(Selection{}, "", dir, map[string]string{"@ID-GONE-90": "stale"}); err == nil || !strings.Contains(err.Error(), "@ID-GONE-90") {
		t.Fatalf("a deferral no scenario carries: %v; want its failure", err)
	}
}

// A deferral by the tag of an Examples block leaves out its rows alone, and
// is told for a selection by its scenario's ID as well as by its own tag,
// never left out silently. dry.feature's scenario, the fixture's other,
// runs "skipped" too, before units.feature's rows, in Converted's order.
func TestDeferredExamplesLeaveOnlyTheirRows(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "")
	dir := featureSelectionFixture(t)
	outline := "Feature: Active file\n  @ID-UNIT-91\n  Scenario Outline: Rows\n    Given the <which> fixture runs\n\n" +
		"    Examples:\n      | which  |\n      | active |\n\n" +
		"    # The binary driver's alone.\n    @T-90\n    Examples:\n      | which   |\n      | skipped |\n"
	if err := os.WriteFile(filepath.Join(dir, "units.feature"), []byte(outline), 0o644); err != nil {
		t.Fatal(err)
	}
	deferred := map[string]string{"@T-90": "a reason naming its owner"}
	for _, c := range []struct {
		pattern   string
		ran, left []string
	}{
		{"", []string{"skipped", "active"}, []string{"@T-90"}},
		{"^@ID-UNIT-91$", []string{"active"}, []string{"@T-90"}},
		{"^@T-90$", nil, []string{"@T-90"}},
		{"^@ID-DRY-90$", []string{"skipped"}, nil},
	} {
		t.Run(c.pattern, func(t *testing.T) {
			sel, err := TagFilter(c.pattern, dir, Converted, InProcessOnly)
			if err != nil {
				t.Fatal(err)
			}
			sel, left, err := Defer(sel, c.pattern, dir, deferred)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(left, c.left) {
				t.Fatalf("deferred left out %v; want %v", left, c.left)
			}
			if ran := runSelection(t, sel.Filter, sel.Paths); !slices.Equal(ran, c.ran) {
				t.Fatalf("scenarios ran: %v; want %v", ran, c.ran)
			}
		})
	}
}
