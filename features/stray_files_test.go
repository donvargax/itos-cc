package features

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// strayDriver is the binary driver's folders and files with a run of itos-cc
// that writes what its first argument says, as a product would: a pytest
// cache in the project, a file in the temporary folder the run was given, or
// nothing.
type strayDriver struct {
	binary
	elsewhere string
}

func (d strayDriver) withTags([]string) Driver { return d }

func (d strayDriver) Run(r Run) (Result, error) {
	temp := d.elsewhere
	for _, kv := range r.Env {
		if value, ok := strings.CutPrefix(kv, "TMPDIR="); ok {
			temp = value
		}
	}
	var path string
	switch r.Args[0] {
	case "cache":
		path = filepath.Join(r.Dir, ".pytest_cache", "v", "cache", "lastfailed")
	case "metrics":
		path = filepath.Join(r.Dir, ".metrics", "coverage", "go.json")
	case "temp":
		path = filepath.Join(temp, "left-by-board-test")
	default:
		return Result{}, nil
	}
	return Result{}, d.Write(path, []byte("left\n"))
}

// TestScenarioLeavesNoStrayFiles runs scenarios in process through the
// binary driver's steps with a product that leaves files, as T-236 holds
// every scenario to what a run leaves: a file new in the project, but under
// .metrics/, or in the run's temporary folder fails the scenario naming its
// path, unless an allowance names the scenario's tag and the path; one that
// leaves nothing passes.
func TestScenarioLeavesNoStrayFiles(t *testing.T) {
	cases := []struct {
		name, tags, command string
		fails               bool
		says                []string
	}{
		{"a pytest cache in the project", "", "cache", true, []string{".pytest_cache/v/cache/lastfailed", "must leave the project as it found it"}},
		{"a file in the temporary folder", "", "temp", true, []string{"left-by-board-test", "temporary folder"}},
		{"a file in the temporary folder, under its allowance", "@ID-MUT-354", "temp", false, nil},
		{"a file under .metrics/", "", "metrics", false, nil},
		{"nothing", "", "nothing", false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			feature := "Feature: stray files\n  " + c.tags + "\n  Scenario: a run\n    When itos-cc runs with \"" + c.command + "\"\n"
			var out bytes.Buffer
			suite := godog.TestSuite{
				Name: "stray files",
				ScenarioInitializer: func(sc *godog.ScenarioContext) {
					initialize(sc, strayDriver{elsewhere: t.TempDir()})
				},
				Options: &godog.Options{
					Format:          "pretty",
					Output:          &out,
					NoColors:        true,
					FeatureContents: []godog.Feature{{Name: "stray.feature", Contents: []byte(feature)}},
					Strict:          true,
					Concurrency:     1,
				},
			}
			status := suite.Run()
			if c.fails != (status != 0) {
				t.Fatalf("the scenario ended with status %d, want failing: %v\n%s", status, c.fails, out.String())
			}
			for _, text := range c.says {
				if !strings.Contains(out.String(), text) {
					t.Errorf("the report does not say %q\n%s", text, out.String())
				}
			}
		})
	}
}
