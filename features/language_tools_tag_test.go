package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// TestLanguageScenariosCarryTheirToolsTag runs, through the binary driver's
// steps and in this process, scenarios whose Givens declare a project's
// coverage or tests in each language, as a CI job that installed no
// language tools runs them (CI set, ITOS_CC_LANGUAGE_TOOLS and
// ITOS_CC_FLUTTER_TOOLS unset): a scenario needing Python, TypeScript or
// Kotlin tools without @language-tools, or Dart's without @flutter-tools,
// fails naming the tag, so a new one never skips unnoticed in every job;
// one with its tag, on the scenario or on the Examples block of its row,
// skips as before, for the job that installs those tools runs it; and Go
// needs no tag. In the languages job, which sets ITOS_CC_LANGUAGE_TOOLS and
// installs no Flutter, a Dart scenario needs @flutter-tools alike (T-232).
func TestLanguageScenariosCarryTheirToolsTag(t *testing.T) {
	cases := []struct {
		tags, language, part string
		want                 toolsTagged
	}{
		{"", "python", "coverage", toolsTagged{godog.StepFailed, "@language-tools"}},
		{"", "typescript", "coverage", toolsTagged{godog.StepFailed, "@language-tools"}},
		{"", "kotlin", "tests", toolsTagged{godog.StepFailed, "@language-tools"}},
		{"", "dart", "coverage", toolsTagged{godog.StepFailed, "@flutter-tools"}},
		{"@language-tools", "dart", "tests", toolsTagged{godog.StepFailed, "@flutter-tools"}},
		{"@language-tools", "python", "tests", toolsTagged{godog.StepSkipped, ""}},
		{"@language-tools", "typescript", "coverage", toolsTagged{godog.StepSkipped, ""}},
		{"@language-tools", "kotlin", "coverage", toolsTagged{godog.StepSkipped, ""}},
		{"@flutter-tools", "dart", "coverage", toolsTagged{godog.StepSkipped, ""}},
		{"", "go", "coverage", toolsTagged{godog.StepPassed, ""}},
		{"", "go", "tests", toolsTagged{godog.StepPassed, ""}},
	}
	var feature strings.Builder
	feature.WriteString("Feature: language tools\n")
	wants := map[string]toolsTagged{}
	for i, c := range cases {
		step := fmt.Sprintf("a %s project's %s, in scenario %d", c.language, c.part, i)
		fmt.Fprintf(&feature, "  %s\n  Scenario: case %d\n    Given %s\n", c.tags, i, step)
		wants[step] = c.want
	}
	// An Examples block's tag stands for the scenario's on the block's rows.
	feature.WriteString("  Scenario Outline: rows\n    Given a <language> project's <part>, in <row>\n" +
		"    @language-tools\n    Examples:\n      | language | part | row |\n      | python | tests | a tagged row |\n" +
		"    Examples:\n      | language | part | row |\n      | typescript | coverage | an untagged row |\n      | go | tests | a row of go |\n")
	wants["a python project's tests, in a tagged row"] = toolsTagged{godog.StepSkipped, ""}
	wants["a typescript project's coverage, in an untagged row"] = toolsTagged{godog.StepFailed, "@language-tools"}
	wants["a go project's tests, in a row of go"] = toolsTagged{godog.StepPassed, ""}

	t.Setenv("CI", "true")
	t.Setenv(languageToolsEnv, "")
	t.Setenv(flutterToolsEnv, "")
	checkToolsTags(t, feature.String(), wants)

	t.Run("in each job", func(t *testing.T) {
		// Each job as the variables it sets say: none, the languages job,
		// the dart job, and Flutter said installed alone, which needs the
		// language tools said installed too, as the dart job says both.
		jobs := []struct{ ci, tools, flutter string }{
			{"", "", ""}, {"true", "", ""}, {"true", "1", ""}, {"true", "1", "1"}, {"true", "", "1"},
		}
		for _, job := range jobs {
			t.Setenv("CI", job.ci)
			t.Setenv(languageToolsEnv, job.tools)
			t.Setenv(flutterToolsEnv, job.flutter)
			for _, language := range []string{"go", "python", "typescript", "kotlin", "dart"} {
				installed := job.ci == "" || language == "go" || job.tools != "" && (language != "dart" || job.flutter != "")
				tag := languageToolsTag
				if language == "dart" {
					tag = flutterToolsTag
				}
				for _, tags := range [][]string{nil, {"@ID-X-1", tag}, {languageToolsTag, flutterToolsTag}} {
					err := languageJob(language, tags)
					switch has := slices.Contains(tags, tag); {
					case installed && err != nil:
						t.Errorf("%+v, %s, %v: %v, want none", job, language, tags, err)
					case !installed && has && !errors.Is(err, godog.ErrSkip):
						t.Errorf("%+v, %s, %v: %v, want a skip", job, language, tags, err)
					case !installed && !has && (err == nil || errors.Is(err, godog.ErrSkip) || !strings.Contains(err.Error(), tag)):
						t.Errorf("%+v, %s, %v: %v, want a failure naming %s", job, language, tags, err, tag)
					}
				}
			}
		}
	})

	t.Run("in the languages job", func(t *testing.T) {
		t.Setenv(languageToolsEnv, "1")
		checkToolsTags(t, "Feature: language tools\n"+
			"  @language-tools\n  Scenario: untagged for flutter\n    Given a dart project's coverage, in the languages job\n"+
			"  @flutter-tools\n  Scenario: tagged for flutter\n    Given a dart project's tests, in the languages job\n",
			map[string]toolsTagged{
				"a dart project's coverage, in the languages job": {godog.StepFailed, "@flutter-tools"},
				"a dart project's tests, in the languages job":    {godog.StepSkipped, ""},
			})
	})
}

// toolsTagged is how a step declaring a language's tools ends: its status,
// and the tag its error names, "" for none.
type toolsTagged struct {
	status godog.StepResultStatus
	tag    string
}

// checkToolsTags runs the feature text through the binary driver's steps,
// with one more that declares a project's coverage or tests in a language,
// and checks each such step ends as wants says, by its text.
func checkToolsTags(t *testing.T, feature string, wants map[string]toolsTagged) {
	t.Helper()
	type ended struct {
		status godog.StepResultStatus
		err    error
	}
	got := map[string]ended{}
	declare := func(sc *godog.ScenarioContext, w *world) {
		sc.Step(`^a (\w+) project's (coverage|tests), in [^,]+$`, func(language, part string) error {
			if part == "coverage" {
				return w.d.Coverage(w.dir, Coverage{Language: language})
			}
			return w.d.Tests(w.dir, Tests{Language: language, Command: "go test"})
		})
		sc.StepContext().After(func(ctx context.Context, st *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
			got[st.Text] = ended{status, err}
			return ctx, nil
		})
	}
	suite := godog.TestSuite{
		Name:                "language tools",
		ScenarioInitializer: func(sc *godog.ScenarioContext) { initialize(sc, binary{}, declare) },
		Options: &godog.Options{
			Format:          "progress",
			Output:          io.Discard,
			FeatureContents: []godog.Feature{{Name: "language_tools.feature", Contents: []byte(feature)}},
			Strict:          true,
			Concurrency:     1,
		},
	}
	suite.Run()
	for text, w := range wants {
		e, ok := got[text]
		switch {
		case !ok:
			t.Errorf("%q: the step never ran", text)
		case e.status != w.status:
			t.Errorf("%q ended %v (%v), want %v", text, e.status, e.err, w.status)
		case w.tag != "" && (e.err == nil || !strings.Contains(e.err.Error(), w.tag)):
			t.Errorf("%q failed with %v, which does not name %s", text, e.err, w.tag)
		}
	}
}
