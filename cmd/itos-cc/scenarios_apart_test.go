package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	gherkin "github.com/cucumber/gherkin/go/v42"
	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/covrun"
)

// The scenarios measured apart, as itos-cc's listed tests (mutation.tests in
// itos-cc.yaml, decision 49): itos-cc runs them all once with
// ITOS_CC_TEST_COVERDIR set and reads each one's coverage from
// <dir>/<ID> (decision 10). A go test -cover binary writes its coverage once,
// as it exits, and only to -test.gocoverdir, never GOCOVERDIR, and
// runtime/coverage cannot clear or write its counters inside one, so
// TestScenarios, run so, runs each scenario it selects in a run of its own
// test binary with -test.gocoverdir=<dir>/<ID>, the ID being the scenario's
// ID tag without its @ (T-184). Without the variable, or without coverage,
// it runs them in this process, as ever.

// idTag is a scenario's ID tag, as itos.yaml's tests.scenario.id has it.
var idTag = regexp.MustCompile(`^@ID-[A-Z]+-\d+$`)

// measuredApart is the folder each scenario's coverage goes under when this
// run measures them apart, "" when it runs them in this process.
func measuredApart() string {
	if testing.CoverMode() == "" {
		return ""
	}
	return os.Getenv(covrun.TestCoverDirEnv)
}

// runApart runs each scenario sel selects in a run of this test binary of
// its own, one at a time, writing its coverage to dir/<ID>. A scenario that
// fails there fails its subtest, and so TestScenarios, with what it printed.
func runApart(t *testing.T, dir string, sel features.Selection) {
	ids, err := selectedIDs(sel)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("each of %d scenarios runs apart, its coverage under %s/<ID> (%s)", len(ids), dir, covrun.TestCoverDirEnv)
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, covrun.TestCoverDirEnv+"=")
	})
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			out := filepath.Join(dir, id)
			if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // the folder itos-cc reads the scenario's coverage from, as it makes its own
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestScenarios$", "-test.gocoverdir="+out, "-scenarios=^@"+regexp.QuoteMeta(id)+"$") //nolint:gosec // this test binary, run again for one scenario
			cmd.Env = env
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Run(); err != nil {
				t.Errorf("%s, run apart: %v\n%s", id, err, output.Bytes())
			}
		})
	}
}

// selectedIDs is the ID of every scenario of sel's files that its filter
// selects, without the @, once each, in the files' order. A selected
// scenario with no ID tag is an error: no run of its own can select it.
func selectedIDs(sel features.Selection) ([]string, error) {
	var ids []string
	for _, path := range sel.Paths {
		data, err := os.ReadFile(path) //nolint:gosec // a feature file of this module, as the selection names it
		if err != nil {
			return nil, err
		}
		n := 0
		newID := func() string { n++; return strconv.Itoa(n) }
		doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), newID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, pickle := range gherkin.Pickles(*doc, path, newID) {
			var tags []string
			for _, tag := range pickle.Tags {
				tags = append(tags, tag.Name)
			}
			if !filterSelects(sel.Filter, tags) {
				continue
			}
			i := slices.IndexFunc(tags, idTag.MatchString)
			if i < 0 {
				return nil, fmt.Errorf("%s: scenario %q has no ID tag, so it cannot run apart", path, pickle.Name)
			}
			if id := strings.TrimPrefix(tags[i], "@"); !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// filterSelects is whether a scenario carrying tags is one godog's tag
// filter selects, as godog reads it: any of its terms, joined by commas,
// each holding when every tag of it, joined by &&, is one the scenario
// carries, or, after ~, one it does not; the @ is left out on both sides.
func filterSelects(filter string, tags []string) bool {
	carries := func(tag string) bool {
		return slices.ContainsFunc(tags, func(t string) bool {
			return strings.ReplaceAll(t, "@", "") == strings.ReplaceAll(tag, "@", "")
		})
	}
	for _, term := range strings.Split(filter, ",") {
		holds := term != ""
		for _, tag := range strings.Split(term, "&&") {
			tag = strings.TrimSpace(tag)
			if not, ok := strings.CutPrefix(tag, "~"); ok {
				holds = holds && !carries(not)
			} else {
				holds = holds && carries(tag)
			}
		}
		if holds {
			return true
		}
	}
	return false
}
