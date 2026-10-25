package features

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// renewSite is the site of the mutant the reviewed exception of renewProject
// excepts, in Label.
const renewSite = "label.go:6:7"

// extensionSteps are slice-55's: an x- key kept inside the entry --renew or
// a replacing mutation except rewrites.
func (e *exceptionMutation) extensionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^that entry in itos-cc\.yaml also holds "([^"]*)"$`, e.holdExtension)
	sc.Step(`^a commit that renames a string literal on another line of that function, and the mutant still survives$`, func(ctx context.Context) error {
		if err := e.renameLiteral(); err != nil {
			return err
		}
		return e.rejudgeRenew(ctx)
	})
	sc.Step(`^the mutant still survives at the same site$`, e.sameSite)
	sc.Step(`^that entry in itos-cc\.yaml still holds "([^"]*)"$`, e.stillHolds)
}

// holdExtension writes line into the reviewed entry, right after its first
// line, so a writer that keeps it moves it after the keys it writes.
func (e *exceptionMutation) holdExtension(line string) error {
	written := e.s.m.read("itos-cc.yaml")
	first := "version: 1\nmutation:\n  exceptions:\n    - file: label.go\n"
	if !strings.HasPrefix(written, first) {
		return fmt.Errorf("unexpected entries as written:\n%s", written)
	}
	return e.s.m.w.write("itos-cc.yaml", first+"      "+line+"\n"+written[len(first):])
}

// sameSite holds that the reviewed mutant still survives, unchanged, and
// gives the command's "that site" as its site.
func (e *exceptionMutation) sameSite() error {
	snap, err := e.s.snapshot("label.go")
	if err != nil {
		return err
	}
	u := e.s.units(snap, renewModule+"#Label")
	if len(u) != 1 || u[0]["survived"] != float64(1) || u[0]["hash"] != e.renewBefore[renewModule+"#Label"]["hash"] {
		return fmt.Errorf("unit Label not surviving unchanged: %v", u)
	}
	w := e.s.m.w
	previous := w.prepare
	w.prepare = func(args []string) []string {
		if previous != nil {
			args = previous(args)
		}
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "that" && args[i+1] == "site" {
				args = slices.Replace(args, i, i+2, renewSite)
			}
		}
		return args
	}
	return nil
}

// stillHolds holds the reviewed entry, which the command rewrote, to every
// key and value of text.
func (e *exceptionMutation) stillHolds(text string) error {
	if e.s.m.w.exit != 0 {
		return fmt.Errorf("the command failed: %s", e.s.m.w.report())
	}
	var want map[string]any
	if err := yaml.Unmarshal([]byte(text), &want); err != nil {
		return err
	}
	a, err := e.entriesByFunction()
	if err != nil {
		return err
	}
	entry := a[renewModule+"#Label"]
	for key, v := range want {
		if !reflect.DeepEqual(entry[key], v) {
			return fmt.Errorf("the entry holds %s: %v, want %v\n%s", key, entry[key], v, e.s.m.read("itos-cc.yaml"))
		}
	}
	own := maps.Clone(entry)
	maps.DeleteFunc(own, func(key string, _ any) bool { return strings.HasPrefix(key, "x-") })
	if reflect.DeepEqual(own, e.renewBefore[renewModule+"#Label"]) {
		return fmt.Errorf("the entry was not rewritten: %v", entry)
	}
	return nil
}
