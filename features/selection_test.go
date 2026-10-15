package features

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// Selection belongs to the test runners, which the product's command line
// cannot exercise. Tiny feature files show which scenarios actually run.
func TestSkippedFeatureSelectionRunsNothing(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "count.feature")
	dir := featureSelectionFixture(t)
	sel, err := TagFilter("^@ID-COUNT-90$", dir, Converted, InProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Filter != "" {
		t.Fatalf("a selection naming only a skipped feature would run scenarios: %s", sel.Filter)
	}
}

func TestMixedFeatureSelectionRunsOnlyActiveScenarios(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "count.feature")
	dir := featureSelectionFixture(t)
	sel, err := TagFilter("^@ID-(COUNT|UNIT)-90$", dir, Converted, InProcessOnly)
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
		t.Fatalf("scenarios ran: %v; want both files", ran)
	}
}

func TestFeatureSelectionRejectsUnknownTagsDespiteSkippedFiles(t *testing.T) {
	t.Setenv("ITOS_CC_SKIP_FEATURES", "count.feature")
	_, err := TagFilter("^@ID-UNKNOWN-90$", featureSelectionFixture(t), Converted, InProcessOnly)
	if err == nil || !strings.Contains(err.Error(), "no scenario has a tag") {
		t.Fatalf("unknown selection: %v; want a no-match failure", err)
	}
}

func TestFeatureSelectionValidatesUnsteppedIDsDespiteSkippedFiles(t *testing.T) {
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

func featureSelectionFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range Converted {
		text := "Feature: " + name + "\n"
		switch name {
		case "count.feature":
			text = "@shared\nFeature: Skipped file\n  @ID-COUNT-90\n  Scenario: Skipped\n    Given the skipped fixture runs\n"
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
