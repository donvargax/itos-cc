package main

import (
	"encoding/json"
	"testing"
)

// Helpers the counted tests share, from the tests of "Rule: A kill made by
// listed tests goes stale when its tests change" in features/mutate.feature,
// which ID-MUT-130 to 134 hold whole (T-101). They use the module of
// mutate_listed_test.go, with what main writes to stderr left out, so no
// mutant survives, and a list that names a file for each test: ID-A-01 is
// defined in c.feature, ID-A-02 in a.feature, and ID-A-03, which the harness
// runs nothing for and so covers nothing, in b.feature. itos-cc.yaml names
// features/*_test.go as support files. ID-A-02 alone kills the mutants of
// greet's line 17; the own test kills half's; twice's is uncovered.

const (
	greetID = "example.com/greet#greet"
	mainID  = "example.com/greet#main"
	halfID  = "example.com/greet#half"
)

// checked runs mutation check --json on main.go and returns what it said
// of each function, by namespace#name: its state, and the message of its
// mutation.stale problem, if any.
func checked(t *testing.T) (map[string]string, map[string]string, outcome) {
	t.Helper()
	o := itosCc(t, "mutation", "check", "--json", greetSource)
	m := o.json(t)
	var out struct {
		Files []struct {
			Functions []struct {
				Function string `json:"function"`
				State    string `json:"state"`
			} `json:"functions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatal(err)
	}
	states, stale := map[string]string{}, map[string]string{}
	for _, f := range out.Files {
		for _, fn := range f.Functions {
			states[fn.Function] = fn.State
		}
	}
	for _, p := range m.Problems {
		if p["rule"] == "mutation.stale" {
			fn, _ := p["function"].(string)
			stale[fn], _ = p["message"].(string)
		}
	}
	return states, stale, o
}
