package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

const (
	sameNameSnapshot        = ".metrics/mutate/board/setup.go.json"
	sameNameID              = "example.com/fx/board#init"
	sameNameFeatureRecorded = "Feature: setup\n"
	sameNameFeatureNow      = "Feature: setup, both init functions\n"
	sameNameSuiteTest       = "e2e/support_test.go"
	sameNameSuiteSource     = "package e2e\n\nimport \"testing\"\n\nfunc TestSupport(t *testing.T) {}\n"
)

func (i *identityException) sameNameRunSteps(sc *godog.ScenarioContext) {
	sc.Step(`^"itos-cc mutation run --all-tests board/setup\.go" recorded both surviving$`, i.wholeSuiteRecorded)
	sc.Step(`^e2e/support_test\.go, a test of another package that the whole suite runs, has changed since$`, func() error {
		return i.w.write(sameNameSuiteTest, sameNameSuiteSource+"\n// TestSupport checks nothing yet.\n")
	})
	sc.Step(`^the only survivor it fails on is the second init function's, at board/setup\.go line 11 column 10$`, i.onlySecondSurvivor)
	sc.Step(`^a commit after "base" changed only the second init function of board/setup\.go$`, i.secondInitChanged)
	sc.Step("^the snapshot records the first init function's mutant at board/setup\\.go:7:10 killed by the listed test ID-A-01, of a\\.feature, which has changed since$", i.firstKilledByListed)
	sc.Step(`^"itos-cc mutation check board/setup\.go" reports the first init function "mutation\.stale", kept by a mutation run that did not judge it$`, i.firstMarkedStale)
}

func (i *identityException) wholeSuiteRecorded() error {
	w := i.w
	if err := w.write(sameNameSuiteTest, sameNameSuiteSource); err != nil {
		return err
	}
	if err := w.git("add", sameNameSuiteTest); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "e2e"); err != nil {
		return err
	}
	surviving := []Site{{File: sameNameFile, Line: 7, Original: "<", Replacement: "<="}, {File: sameNameFile, Line: 11, Original: "<", Replacement: "<="}}
	for _, command := range []string{"go test -count=1 -failfast ./...", "go test -count=1 ./..."} {
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: command, Surviving: surviving}); err != nil {
			return err
		}
	}
	if err := w.runWith(w.env(), "mutation run --all-tests "+sameNameFile); err != nil {
		return err
	}
	if w.exit != 1 {
		return fmt.Errorf("mutation run --all-tests exited %d, not 1 for two survivors\n%s", w.exit, w.report())
	}
	return nil
}

func (i *identityException) onlySecondSurvivor() error {
	var survivors []string
	for _, line := range outputLines(i.w.stdout) {
		if strings.HasPrefix(strings.TrimSpace(line), "survived ") {
			survivors = append(survivors, strings.TrimSpace(line))
		}
	}
	want := "survived `<` → `<=` in " + sameNameID + ", board/setup.go line 11 column 10"
	if len(survivors) != 1 || !strings.Contains(strings.ReplaceAll(survivors[0], `\`, "/"), want) {
		return fmt.Errorf("the run fails on the survivors %q, not %q alone\n%s", survivors, want, i.w.report())
	}
	return nil
}

func (i *identityException) secondInitChanged() error {
	w := i.w
	if err := w.git("tag", "base"); err != nil {
		return err
	}
	changed := strings.Replace(sameNameSource, "Top = 5 < 6", "Top = 5 < 7", 1)
	if changed == sameNameSource {
		return fmt.Errorf("%s has no second init function to change", sameNameFile)
	}
	if err := w.write(sameNameFile, changed); err != nil {
		return err
	}
	if err := w.git("add", sameNameFile); err != nil {
		return err
	}
	return w.git("commit", "-qm", "second init")
}

func (i *identityException) firstKilledByListed() error {
	w := i.w
	if err := w.write("a.feature", sameNameFeatureNow); err != nil {
		return err
	}
	data, err := w.readIn(w.dir, sameNameSnapshot)
	if err != nil {
		return err
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	units, _ := snap["units"].([]any)
	listed := false
	for _, one := range units {
		u, _ := one.(map[string]any)
		if u["start_line"] != float64(6) {
			continue
		}
		mutants, _ := u["mutants"].([]any)
		for _, each := range mutants {
			m, _ := each.(map[string]any)
			if m["line"] == float64(7) && m["column"] == float64(10) && m["original"] == "<" && m["replacement"] == "<=" && m["outcome"] == "survived" {
				m["outcome"], m["scope"], m["tests"] = "killed", "listed", []string{"ID-A-01"}
				u["killed"], u["survived"] = u["killed"].(float64)+1, u["survived"].(float64)-1
				listed = true
			}
		}
	}
	if !listed {
		return fmt.Errorf("the snapshot records no survivor at board/setup.go:7:10 in the first init function:\n%s", data)
	}
	before := sha256.Sum256([]byte(sameNameFeatureRecorded))
	snap["listed"] = map[string]string{"ID-A-01": "a.feature"}
	snap["listed_files"] = map[string]string{"a.feature": hex.EncodeToString(before[:])}
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return w.write(sameNameSnapshot, string(out)+"\n")
}

func (i *identityException) firstMarkedStale() error {
	w := i.w
	code, out, errOut, err := w.capture(w.dir, w.env(), "mutation", "check", "--json", sameNameFile)
	if err != nil {
		return err
	}
	report := fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, out, errOut)
	var object map[string]any
	if err := json.Unmarshal([]byte(out), &object); err != nil {
		return fmt.Errorf("mutation check printed no JSON object: %v\n%s", err, report)
	}
	problems, _ := object["problems"].([]any)
	for _, one := range problems {
		p, _ := one.(map[string]any)
		message, _ := p["message"].(string)
		if p["rule"] == "mutation.stale" && p["function"] == sameNameID && p["line"] == float64(6) &&
			strings.Contains(message, "kept by a mutation run that did not judge it") {
			return nil
		}
	}
	return fmt.Errorf("mutation check reports no \"mutation.stale\" for the first init function, at line 6\n%s", report)
}
