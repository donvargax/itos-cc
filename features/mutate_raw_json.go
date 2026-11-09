package features

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of slice-77's scenarios: what --json's text says before anything
// decodes it, as a person or a script copying from the raw text reads it. They
// read w.stdout as it is, and a problem's values as json.RawMessage, which
// keeps a string's bytes with its escapes undecoded.

// rawExcept is the identity in the mutation except command a fix names.
var rawExcept = regexp.MustCompile(`mutation except '([^']*)'`)

func (i *identityException) rawJSONSteps(sc *godog.ScenarioContext) {
	sc.Step(`^stdout's raw text holds "([^"]*)" and "([^"]*)"$`, func(first, second string) error { return i.rawHolds(first, second) })
	sc.Step(`^stdout's raw text holds none of "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(first, second, third string) error { return i.rawHoldsNone(first, second, third) })
	sc.Step(`^I except the identity read from the raw text of the "mutation\.survived" problem's fix, with no JSON decoding, with --reason 'x is never 5'$`, i.exceptRawIdentity)
}

func (i *identityException) rawHolds(texts ...string) error {
	for _, text := range texts {
		if !strings.Contains(i.w.stdout, text) {
			return fmt.Errorf("stdout's raw text does not hold %q\n%s", text, i.w.report())
		}
	}
	return nil
}

func (i *identityException) rawHoldsNone(texts ...string) error {
	for _, text := range texts {
		if strings.Contains(i.w.stdout, text) {
			return fmt.Errorf("stdout's raw text holds %q\n%s", text, i.w.report())
		}
	}
	return nil
}

// exceptRawIdentity runs mutation except with the identity as the raw text of
// the survivor's fix has it, as one argument, so nothing unquotes it either.
func (i *identityException) exceptRawIdentity() error {
	w := i.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	var object struct {
		Problems []map[string]json.RawMessage `json:"problems"`
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("stdout is not one JSON object: %w\n%s", err, w.report())
	}
	for _, p := range object.Problems {
		if string(p["rule"]) != `"mutation.survived"` {
			continue
		}
		found := rawExcept.FindSubmatch(p["fix"])
		if found == nil {
			return fmt.Errorf("the raw fix %s names no mutation except\n%s", p["fix"], w.report())
		}
		return w.runEnv(w.dir, w.env(), "mutation", "except", string(found[1]), "--reason", identityReason)
	}
	return fmt.Errorf("no mutation.survived problem\n%s", w.report())
}
