package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/donvargax/itos-cc/features"
)

// leavingMemory is the in-process driver with a run of itos-cc that writes
// the one file its second argument names in the project, as a product would.
type leavingMemory struct{ *memory }

func (d leavingMemory) Run(r features.Run) (features.Result, error) {
	return features.Result{}, d.Write(filepath.Join(r.Dir, r.Args[2]), []byte("left\n"))
}

// TestScenarioLeavesNoStrayFilesInProcess holds the in-process driver to the
// same rule as the binary one (T-240): the report a scenario names with
// --coverage-report is let by, and any other new file is refused by name.
func TestScenarioLeavesNoStrayFilesInProcess(t *testing.T) {
	cases := []struct {
		name, command string
		fails         bool
	}{
		{"the report the scenario names", "report --coverage-report named.out", false},
		{"a file the scenario does not name", "report --other stray.out", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			feature := "Feature: stray files\n  Scenario: a run\n    When itos-cc runs with \"" + c.command + "\"\n"
			var out bytes.Buffer
			suite := godog.TestSuite{
				Name:                "stray files in process",
				ScenarioInitializer: func(sc *godog.ScenarioContext) { features.Initialize(sc, leavingMemory{newMemory(t)}) },
				Options: &godog.Options{
					Format: "pretty", Output: &out, NoColors: true, Strict: true, Concurrency: 1,
					FeatureContents: []godog.Feature{{Name: "stray.feature", Contents: []byte(feature)}},
				},
			}
			if status := suite.Run(); c.fails != (status != 0) {
				t.Fatalf("status %d, want failing: %v\n%s", status, c.fails, out.String())
			}
			if c.fails && !strings.Contains(out.String(), "stray.out") {
				t.Errorf("the report does not name stray.out\n%s", out.String())
			}
		})
	}
}
