package features

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// slice-39: a listed kill is dated by its test's fingerprint, the third
// column the list command may print, when it prints one (MUT-276 to 279).
// The list's output is the fixture's testdata/list.go, which the binary
// driver runs, and the in-process lister prints (ListedTests.ListOutput).

// The lists the scenarios set: the fresh project's (freshProject), ID-A-02
// with a fingerprint, then changed, moved, unlisted and without one.
const (
	slice39Printed   = "ID-A-01\tc.feature\nID-A-02\ta.feature\tf1\nID-A-03\tb.feature\n"
	slice39Moved     = "ID-A-01\tc.feature\nID-A-02\tb.feature\tf1\nID-A-03\tb.feature\n"
	slice39Changed   = "ID-A-01\tc.feature\nID-A-02\ta.feature\tf2\nID-A-03\tb.feature\n"
	slice39Unlisted  = "ID-A-01\tc.feature\nID-A-03\tb.feature\n"
	slice39Unprinted = "ID-A-01\tc.feature\nID-A-02\ta.feature\nID-A-03\tb.feature\n"
	slice39Mixed     = "ID-A-02\ta.feature\tf1\nID-B-01\tb.feature\n"
)

var slice39PrintLine = regexp.MustCompile(`fmt\.Print\(".*"\)`)

func slice39FingerprintSteps(sc *godog.ScenarioContext, l *listedMutation) {
	sc.Step(`^mutation\.tests\.list prints "ID-A-02", "a\.feature" and the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills$`, func() error {
		if err := l.slice39Recorded(slice39Printed); err != nil {
			return err
		}
		return l.slice39RecordedIn(map[string]string{"ID-A-02": "a.feature"})
	})
	sc.Step(`^a\.feature has changed since, in a comment and in another scenario, and the list still prints "f1" for ID-A-02$`, func() error {
		return l.m.w.write("a.feature", l.m.read("a.feature")+"# why the next scenario is here\n\nScenario: another\n  Given something else\n")
	})
	sc.Step(`^after ID-A-02 moves to b\.feature, still listed with "f1", the kill is still fresh$`, l.slice39MovedFresh)
	sc.Step(`^mutation\.tests\.list prints the fingerprint "f1" for ID-A-02, and a run recorded a mutant ID-A-02 kills$`, func() error { return l.slice39Recorded(slice39Printed) })
	sc.Step(`^the list now prints "f2" for ID-A-02$`, func() error { return l.slice39Edited(slice39Changed) })
	sc.Step(`^the problem is "mutation\.stale" for that mutant's function, and its message names ID-A-02$`, func() error { return l.stale([]string{"greet"}, "ID-A-02") })
	sc.Step(`^when the list no longer prints ID-A-02 at all, the kill is stale the same way$`, l.slice39UnlistedStale)
	sc.Step(`^mutation\.tests\.list prints a fingerprint for ID-A-02 in a\.feature and none for ID-B-01 in b\.feature$`, l.slice39MixedList)
	sc.Step(`^a run recorded a mutant ID-A-02 kills and another ID-B-01 kills$`, l.slice39MixedRecorded)
	sc.Step(`^a\.feature and b\.feature change elsewhere, the list's fingerprint for ID-A-02 unchanged$`, func() error {
		if err := l.edit("a.feature"); err != nil {
			return err
		}
		return l.edit("b.feature")
	})
	sc.Step(`^"itos-cc mutation check" reports ID-B-01's kill "mutation\.stale", naming b\.feature, and ID-A-02's fresh$`, l.slice39MixedCheck)
	sc.Step(`^after features/steps_test\.go changes, both kills are "mutation\.stale", naming it$`, func() error {
		if err := l.edit("features/steps_test.go"); err != nil {
			return err
		}
		if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
			return err
		}
		return l.stale([]string{"greet"}, "features/steps_test.go")
	})
	sc.Step(`^a run recorded a mutant ID-A-02 kills, ID-A-02 listed with a fingerprint in a\.feature$`, func() error { return l.slice39Recorded(slice39Printed) })
	sc.Step(`^a\.feature has changed since, and the list command now fails$`, func() error {
		if err := l.edit("a.feature"); err != nil {
			return err
		}
		return l.slice39ListFails(true)
	})
	sc.Step(`^the problem is "tests\.list-failed", with its exit code, and the exit code is 1$`, func() error {
		if err := l.listProblem(); err != nil {
			return err
		}
		return l.m.w.exitsWith(1)
	})
	sc.Step(`^with a\.feature unchanged, mutation check runs no list command and the exit code is 0$`, l.slice39UnchangedNoList)
	sc.Step(`^when every recorded listed kill was listed without a fingerprint, mutation check runs no list command$`, l.slice39UnprintedNoList)
}

// slice39SetList makes the list command print out, in both drivers.
func (l *listedMutation) slice39SetList(out string) error {
	text := l.m.read("testdata/list.go")
	if !slice39PrintLine.MatchString(text) {
		return fmt.Errorf("testdata/list.go prints no list:\n%s", text)
	}
	if err := l.m.w.write("testdata/list.go", slice39PrintLine.ReplaceAllLiteralString(text, "fmt.Print("+strconv.Quote(out)+")")); err != nil {
		return err
	}
	l.slice39List = out
	return l.declare()
}

// slice39ListFails makes the list command fail, or succeed again.
func (l *listedMutation) slice39ListFails(fail bool) error {
	from, to := "list: go run ./testdata/list.go\n", "list: go run ./testdata/list.go fail\n"
	if !fail {
		from, to = to, from
	}
	if err := l.replace("itos-cc.yaml", from, to); err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	return l.clearRecords()
}

// slice39Recorded is the fresh project, its list printing out, and a run
// that recorded its kills (freshRun).
func (l *listedMutation) slice39Recorded(out string) error {
	if err := l.freshProject(); err != nil {
		return err
	}
	if err := l.slice39SetList(out); err != nil {
		return err
	}
	return l.freshRun()
}

// slice39RecordedIn is whether the snapshot the run wrote records each
// test of files defined in its file, as the list printed it.
func (l *listedMutation) slice39RecordedIn(files map[string]string) error {
	snap, err := l.snapshot()
	if err != nil {
		return err
	}
	listed, _ := snap["listed"].(map[string]any)
	for id, file := range files {
		if listed[id] != file {
			return fmt.Errorf("the snapshot records %s in %q, not %q: %v", id, listed[id], file, listed)
		}
	}
	return nil
}

// slice39Edited changes a.feature, ID-A-02's text among it, and makes the
// list print out.
func (l *listedMutation) slice39Edited(out string) error {
	if err := l.edit("a.feature"); err != nil {
		return err
	}
	if err := l.slice39SetList(out); err != nil {
		return err
	}
	return l.clearRecords()
}

func (l *listedMutation) slice39MovedFresh() error {
	if err := l.m.w.write("a.feature", "Feature: a\n"); err != nil {
		return err
	}
	if err := l.m.w.write("b.feature", l.m.read("b.feature")+"\nScenario: ID-A-02, moved here\n"); err != nil {
		return err
	}
	if err := l.slice39SetList(slice39Moved); err != nil {
		return err
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
		return err
	}
	return l.m.w.exitsWith(0)
}

func (l *listedMutation) slice39UnlistedStale() error {
	if err := l.slice39Edited(slice39Unlisted); err != nil {
		return err
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
		return err
	}
	if err := l.m.w.exitsWith(1); err != nil {
		return err
	}
	return l.stale([]string{"greet"}, "ID-A-02")
}

// slice39MixedList lists ID-A-02 with a fingerprint and, in ID-A-01's
// place, ID-B-01 without one: the harness's test that ID-A-01 was.
func (l *listedMutation) slice39MixedList() error {
	if err := l.freshProject(); err != nil {
		return err
	}
	l.slice39B = true
	if err := l.replace("e2e/e2e_test.go", `{"ID-A-01", []string{"x"}, "one"}`, `{"ID-B-01", []string{"x"}, "one"}`); err != nil {
		return err
	}
	return l.slice39SetList(slice39Mixed)
}

func (l *listedMutation) slice39MixedRecorded() error {
	if err := l.freshRun(); err != nil {
		return err
	}
	entries, err := l.entries()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedOneLine, func(e map[string]any) bool {
		return e["outcome"] == "killed" && e["scope"] == "listed" && slices.Equal(entryIDs(e), []string{"ID-B-01"})
	}); err != nil {
		return err
	}
	return l.slice39RecordedIn(map[string]string{"ID-A-02": "a.feature", "ID-B-01": "b.feature"})
}

// slice39MixedCheck finds greet, which holds both kills, stale for
// b.feature alone, and a run then reuses ID-A-02's kill and runs
// ID-B-01's again.
func (l *listedMutation) slice39MixedCheck() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
		return err
	}
	if err := l.stale([]string{"greet"}, "b.feature"); err != nil {
		return err
	}
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, p := range problems {
		message := fmt.Sprint(p.(map[string]any)["message"])
		if strings.Contains(message, "ID-A-02") || strings.Contains(message, "a.feature") {
			return fmt.Errorf("a problem names ID-A-02's kill, whose fingerprint is unchanged: %s\n%s", message, l.m.w.report())
		}
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return e["reused"] == true && e["scope"] == "listed" }); err != nil {
		return err
	}
	return l.assertLines(entries, listedOneLine, func(e map[string]any) bool { return e["reused"] != true && e["outcome"] == "killed" })
}

// slice39Listed is how many times the list command ran since the records
// were last cleared.
func (l *listedMutation) slice39Listed() (int, error) {
	records, err := l.records()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, record := range records {
		if record["test"] == "list" {
			n++
		}
	}
	return n, nil
}

func (l *listedMutation) slice39NoList() error {
	n, err := l.slice39Listed()
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("mutation check ran the list command %d times, want none\n%s", n, l.m.w.report())
	}
	return nil
}

func (l *listedMutation) slice39UnchangedNoList() error {
	if err := l.m.w.write("a.feature", l.m.held["a.feature"]); err != nil {
		return err
	}
	if err := l.clearRecords(); err != nil {
		return err
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
		return err
	}
	if err := l.m.w.exitsWith(0); err != nil {
		return err
	}
	return l.slice39NoList()
}

// slice39UnprintedNoList records the kills again with a list that prints
// no fingerprint, then changes a.feature and makes the list fail: the
// kill is dated by its file, and no list command runs.
func (l *listedMutation) slice39UnprintedNoList() error {
	if err := l.slice39ListFails(false); err != nil {
		return err
	}
	if err := l.slice39SetList(slice39Unprinted); err != nil {
		return err
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json --mutate-all main.go"); err != nil {
		return err
	}
	if err := l.slice39RecordedIn(map[string]string{"ID-A-02": "a.feature"}); err != nil {
		return err
	}
	if err := l.edit("a.feature"); err != nil {
		return err
	}
	if err := l.slice39ListFails(true); err != nil {
		return err
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json"); err != nil {
		return err
	}
	if err := l.slice39NoList(); err != nil {
		return err
	}
	if err := l.noProblem("tests.list-failed"); err != nil {
		return err
	}
	return l.stale([]string{"greet"}, "a.feature")
}

// slice39Rename gives the listed test from the name to, in every
// declaration of listed.
func slice39Rename(listed *ListedTests, from, to string) {
	if listed == nil {
		return
	}
	rename := func(ids []string) []string {
		out := slices.Clone(ids)
		for i, id := range out {
			if id == from {
				out[i] = to
			}
		}
		return out
	}
	if file, ok := listed.Files[from]; ok {
		delete(listed.Files, from)
		listed.Files[to] = file
	}
	for _, lines := range listed.Lines {
		for line, ids := range lines {
			lines[line] = rename(ids)
		}
	}
	if elapsed, ok := listed.Elapsed[from]; ok {
		delete(listed.Elapsed, from)
		listed.Elapsed[to] = elapsed
	}
	for _, sites := range []map[string][]Site{listed.Surviving, listed.TimingOut} {
		if held, ok := sites[from]; ok {
			delete(sites, from)
			sites[to] = held
		}
	}
	listed.Failing = rename(listed.Failing)
}
