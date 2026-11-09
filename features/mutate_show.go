package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "mutation show says where a mutant is and what it
// changes, running nothing" (slice-73, issue #82), over slice-41's fixture
// (mutate_except_identity.go), read from what itos-cc prints and writes.

func (i *identityException) showSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a comment was added above Count's return since the report$`, func() error {
		return i.edit(func(text string) string {
			return strings.Replace(text, "\treturn", "\t// five itself is out\n\treturn", 1)
		})
	})
	sc.Step(`^with --json, "mutant" is exactly:$`, i.mutantIs)
	sc.Step(`^the problem is "site\.unmatched", its argument the identity and its why "([^"]*)"$`, i.showUnmatched)
}

// mutantIs runs the last run again with --json, which must exit 0 with
// "mutant" the object text holds, no key more or less.
func (i *identityException) mutantIs(text *godog.DocString) error {
	var want any
	if err := json.Unmarshal([]byte(text.Content), &want); err != nil {
		return err
	}
	object, err := i.rerun()
	if err != nil {
		return err
	}
	if i.w.exit != 0 || !reflect.DeepEqual(object["mutant"], want) {
		return fmt.Errorf("\"mutant\" is not %v\n%s", want, i.w.report())
	}
	return nil
}

// showUnmatched holds the problem to site.unmatched, naming the run's
// identity and why, with the function's hash now exactly when it changed.
func (i *identityException) showUnmatched(why string) error {
	id, err := i.identity()
	if err != nil {
		return err
	}
	w := i.w
	if err := w.problemIs("site.unmatched", "argument", id); err != nil {
		return err
	}
	if w.problem["why"] != why {
		return fmt.Errorf("the problem %v does not say why %q", w.problem, why)
	}
	hash, hashed := w.problem["hash"].(string)
	if changed := why == "changed"; hashed != changed || changed && (hash == "" || hash == identityUnit.FindStringSubmatch(id)[3]) {
		return fmt.Errorf("the problem %v should hold the function's hash now exactly when it changed", w.problem)
	}
	return nil
}
