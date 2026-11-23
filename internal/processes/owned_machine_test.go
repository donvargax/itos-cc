package processes

import (
	"os"
	"strings"
	"testing"
)

// TestOwnedCommandTransitionsMatchTheDocument holds the code's transition
// table to docs/state-machines.md's: the same events, states and targets.
func TestOwnedCommandTransitionsMatchTheDocument(t *testing.T) {
	text, err := os.ReadFile("../../docs/state-machines.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(text), "<!-- transitions -->")
	if !ok {
		t.Fatal("docs/state-machines.md has no <!-- transitions --> table")
	}
	table, _, ok := strings.Cut(after, "<!-- /transitions -->")
	if !ok {
		t.Fatal("docs/state-machines.md's transitions table is not closed")
	}
	var documented []string
	for _, line := range strings.Split(table, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| event ") {
			documented = append(documented, line)
		}
	}
	code := ownedTable()
	if len(documented) != len(code) {
		t.Fatalf("the document lists %d transitions, the code %d:\ndocument:\n%s\ncode:\n%s",
			len(documented), len(code), strings.Join(documented, "\n"), strings.Join(code, "\n"))
	}
	sortedDocumented := append([]string(nil), documented...)
	sortLines(sortedDocumented)
	for i := range code {
		if sortedDocumented[i] != code[i] {
			t.Errorf("the document has\n  %s\nwhere the code has\n  %s", sortedDocumented[i], code[i])
		}
	}
}

func sortLines(lines []string) {
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j] < lines[j-1]; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
}

// TestOwnedCommandNextIsATable holds next to its table: over every state
// and event, one row at most applies, an event with no row is no
// transition, and a row sets exactly what it names.
func TestOwnedCommandNextIsATable(t *testing.T) {
	for _, run := range []ownedRun{runWaiting, runCleaning, runReturned} {
		for _, ctx := range []ownedContext{contextLive, contextByParent, contextByTimeout} {
			for _, deadline := range []ownedDeadline{deadlineBefore, deadlinePast} {
				for _, group := range []ownedGroup{groupUnknown, groupLive, groupGone} {
					state := ownedState{run, ctx, deadline, group}
					for _, event := range allOwnedEvents() {
						matching := 0
						for _, row := range ownedTransitions {
							if row.event == event && row.when.holds(state) {
								matching++
							}
						}
						got, ok := next(state, event)
						if matching > 1 {
							t.Errorf("%v: %q has %d rows", state, event, matching)
						}
						if ok != (matching == 1) {
							t.Errorf("%v: %q allowed = %v with %d rows", state, event, ok, matching)
						}
						if !ok && got != state {
							t.Errorf("%v: %q is no transition but moved the state to %v", state, event, got)
						}
					}
				}
			}
		}
	}
	got, ok := next(ownedStart, eventWaitReturns)
	if want := (ownedState{runCleaning, contextLive, deadlineBefore, groupUnknown}); !ok || got != want {
		t.Errorf("next(start, Wait reaps the leader) = %v, %v; want %v", got, ok, want)
	}
	if _, ok := next(got, eventWaitReturns); ok {
		t.Error("Wait reaped the leader twice")
	}
	if _, ok := next(ownedStart, eventCleanupSucceeds); ok {
		t.Error("a cleanup succeeded before its leader was reaped")
	}
}

func allOwnedEvents() []ownedEvent {
	return []ownedEvent{eventContextEndsByParent, eventContextEndsByTimeout, eventWaitReturns, eventGroupFoundLive,
		eventGroupFoundGone, eventDeadlinePasses, eventCleanupSucceeds, eventCleanupFails}
}
