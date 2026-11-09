package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "A green run's --json says what it judged" (slice-72,
// issue #75). They reuse slice-41's fixture and slice-63's, and read only the
// --json object itos-cc prints.

// proofSummarySteps holds what the scenarios of the summary add to slice-41's
// and slice-63's steps.
func (i *identityException) proofSummarySteps(sc *godog.ScenarioContext) {
	w := i.w
	sc.Step(`^"itos_proof" is exactly:$`, func(want *godog.DocString) error { return w.proofIs(want.Content) })
	sc.Step(`^that commit is tagged "base"$`, func() error { return w.git("tag", "base") })
	sc.Step(`^"files" lists board/board\.go with "ran" (\d+) and "reused" (\d+)$`, func(ran, reused int) error {
		return w.fileRan(identityFile, ran, reused)
	})
}

// numbered reads text as one JSON value keeping each number's token as
// written, so 1 differs from 1.0 and 1e0, which itos refuses in a summary.
func numbered(text string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// proofIs is whether the last run's --json object holds the member itos_proof
// at its top, equal to want key by key: a key missing, a null against a number
// and a number written otherwise each fail.
func (w *world) proofIs(want string) error {
	if _, _, err := w.lastObject(); err != nil {
		return err
	}
	expected, err := numbered(want)
	if err != nil {
		return fmt.Errorf("the scenario's summary is not JSON: %w", err)
	}
	got, err := numbered(w.stdout)
	if err != nil {
		return fmt.Errorf("the output is not JSON: %w\n%s", err, w.report())
	}
	object, ok := got.(map[string]any)
	if !ok {
		return fmt.Errorf("the output is not one JSON object\n%s", w.report())
	}
	member, ok := object["itos_proof"]
	if !ok {
		return fmt.Errorf("the object holds no itos_proof\n%s", w.report())
	}
	if !reflect.DeepEqual(member, expected) {
		return fmt.Errorf("itos_proof is %v, want %v\n%s", member, expected, w.report())
	}
	return nil
}

// fileRan is whether the last run's "files" lists file with those counts of
// mutants run and reused.
func (w *world) fileRan(file string, ran, reused int) error {
	object, report, err := w.lastObject()
	if err != nil {
		return err
	}
	f, err := languageRunFile(object, file)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, report)
	}
	if f["ran"] != float64(ran) || f["reused"] != float64(reused) {
		return fmt.Errorf("%s has ran %v and reused %v, want %d and %d\n%s", file, f["ran"], f["reused"], ran, reused, report)
	}
	return nil
}
