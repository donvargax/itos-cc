package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
	"github.com/donvargax/itos-cc/features"
)

// noCountedRuns is the memory driver on a system that makes no counted run:
// its Counted skips, as it does on Windows.
type noCountedRuns struct{ *memory }

func (noCountedRuns) Counted() error {
	return fmt.Errorf("%w: this driver makes no counted run, as on Windows", godog.ErrSkip)
}

// The stale-entry scenarios hold on a system that makes no counted run, which
// a green job cannot show, since godog counts a skip as passed. So a Counted on
// their paths fails here, on any system, not silently on Windows (T-231). This
// reads the cucumber report, and every step must have passed.
func TestStaleEntryScenariosNeedNoCountedRun(t *testing.T) {
	var report bytes.Buffer
	suite := godog.TestSuite{
		Name: "itos-cc stale entries with no counted run",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			features.Initialize(sc, noCountedRuns{newMemory(t)})
		},
		Options: &godog.Options{
			Format: "cucumber",
			Output: &report,
			Paths:  []string{filepath.Join("..", "..", "features", "mutate.feature")},
			// ID-MUT-422 is left out as its commands are the counted and
			// fail-fast runs Windows refuses (ID-MUT-248): it skips there on
			// purpose.
			Tags:        "@slice-76,@slice-78&&~@ID-MUT-422&&~@wip,@slice-80&&~@wip",
			Strict:      true,
			Concurrency: 1,
		},
	}
	status := suite.Run()
	var cucumber []struct {
		Elements []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Steps []struct {
				Name   string `json:"name"`
				Result struct {
					Status string `json:"status"`
				} `json:"result"`
			} `json:"steps"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(report.Bytes(), &cucumber); err != nil {
		t.Fatalf("the cucumber report does not decode: %v\n%s", err, report.String())
	}
	ran := 0
	for _, feature := range cucumber {
		for _, element := range feature.Elements {
			if element.Type != "scenario" {
				continue
			}
			ran++
			for _, step := range element.Steps {
				if step.Result.Status != "passed" {
					t.Errorf("%s: the step %q is %s", element.Name, step.Name, step.Result.Status)
				}
			}
		}
	}
	if ran == 0 {
		t.Error("no scenario ran")
	}
	if status != 0 {
		t.Errorf("godog's status is %d", status)
	}
	t.Logf("%d scenarios ran", ran)
}
