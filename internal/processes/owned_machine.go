package processes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The owned command's cancel, exit, reap and cleanup machine, as
// docs/state-machines.md's transition table has it. A command's run is a
// state of four components, and one function, next, takes a state and an
// event to the next state: an event the table does not list for the state
// it is met in is no transition, so one the document does not list cannot
// be written. TestOwnedCommandTransitionsMatchTheDocument holds the table
// to the document's.

// ownedRun is where a run's Wait stands: pending, reaped and the group
// being cleaned up, or returned.
type ownedRun string

const (
	runWaiting  ownedRun = "waiting"
	runCleaning ownedRun = "cleaning"
	runReturned ownedRun = "returned"
)

// ownedContext is how the command's context stands.
type ownedContext string

const (
	contextLive      ownedContext = "live"
	contextByParent  ownedContext = "ended by its parent"
	contextByTimeout ownedContext = "ended by its timeout"
)

// ownedDeadline is the cleanup's deadline: before it, or past it.
type ownedDeadline string

const (
	deadlineBefore ownedDeadline = "before"
	deadlinePast   ownedDeadline = "past"
)

// ownedGroup is what the cleanup's probe last found of the group: not yet
// asked, something in it that keeps it, or nothing left that can.
type ownedGroup string

const (
	groupUnknown ownedGroup = "unknown"
	groupLive    ownedGroup = "live"
	groupGone    ownedGroup = "gone"
)

// ownedState is a run's state: one value of each component.
type ownedState struct {
	run      ownedRun
	context  ownedContext
	deadline ownedDeadline
	group    ownedGroup
}

// ownedStart is the state a run begins in.
var ownedStart = ownedState{run: runWaiting, context: contextLive, deadline: deadlineBefore, group: groupUnknown}

// ownedEvent is what happens to a run.
type ownedEvent string

const (
	eventContextEndsByParent  ownedEvent = "the context ends by its parent"
	eventContextEndsByTimeout ownedEvent = "the context ends by its timeout"
	eventWaitReturns          ownedEvent = "Wait reaps the leader"
	eventGroupFoundLive       ownedEvent = "the probe finds the group live"
	eventGroupFoundGone       ownedEvent = "the probe finds the group gone"
	eventDeadlinePasses       ownedEvent = "the deadline passes"
	eventCleanupSucceeds      ownedEvent = "the cleanup succeeds"
	eventCleanupFails         ownedEvent = "the cleanup fails"
)

// ownedComponents are the names a table row gives a state's components, in
// the order its row writes them.
var ownedComponents = []string{"run", "context", "deadline", "group"}

// ownedPattern is some of a state's components, by name: what a row
// requires of a state and what it sets.
type ownedPattern map[string]string

// ownedRow is one transition: the event, what it needs of the state, and
// what it sets.
type ownedRow struct {
	event ownedEvent
	when  ownedPattern
	then  ownedPattern
}

// ownedTransitions is the whole table, the only way a state changes.
var ownedTransitions = []ownedRow{
	{eventContextEndsByParent, ownedPattern{"context": string(contextLive)}, ownedPattern{"context": string(contextByParent)}},
	{eventContextEndsByTimeout, ownedPattern{"context": string(contextLive)}, ownedPattern{"context": string(contextByTimeout)}},
	{eventWaitReturns, ownedPattern{"run": string(runWaiting)}, ownedPattern{"run": string(runCleaning)}},
	{eventGroupFoundLive, ownedPattern{"run": string(runCleaning)}, ownedPattern{"group": string(groupLive)}},
	{eventGroupFoundGone, ownedPattern{"run": string(runCleaning)}, ownedPattern{"group": string(groupGone)}},
	{eventDeadlinePasses, ownedPattern{"run": string(runCleaning), "deadline": string(deadlineBefore)}, ownedPattern{"deadline": string(deadlinePast)}},
	{eventCleanupSucceeds, ownedPattern{"run": string(runCleaning), "group": string(groupGone)}, ownedPattern{"run": string(runReturned)}},
	{eventCleanupFails, ownedPattern{"run": string(runCleaning)}, ownedPattern{"run": string(runReturned)}},
}

// String writes a state as its pattern would: every component and its value.
func (s ownedState) String() string {
	return ownedPattern{"run": string(s.run), "context": string(s.context), "deadline": string(s.deadline), "group": string(s.group)}.String()
}

func (s ownedState) get(component string) string {
	switch component {
	case "run":
		return string(s.run)
	case "context":
		return string(s.context)
	case "deadline":
		return string(s.deadline)
	default:
		return string(s.group)
	}
}

func (s *ownedState) set(component, value string) {
	switch component {
	case "run":
		s.run = ownedRun(value)
	case "context":
		s.context = ownedContext(value)
	case "deadline":
		s.deadline = ownedDeadline(value)
	default:
		s.group = ownedGroup(value)
	}
}

// next is the state an event takes a state to, and whether the table has
// the transition at all.
func next(state ownedState, event ownedEvent) (ownedState, bool) {
	return nextIn(ownedTransitions, state, event)
}

// nextIn is next over a table of its own, which a test prunes to play a
// transition the real table has.
func nextIn(table []ownedRow, state ownedState, event ownedEvent) (ownedState, bool) {
	for _, row := range table {
		if row.event == event && row.when.holds(state) {
			for component, value := range row.then {
				state.set(component, value)
			}
			return state, true
		}
	}
	return state, false
}

func (p ownedPattern) holds(state ownedState) bool {
	for component, value := range p {
		if state.get(component) != value {
			return false
		}
	}
	return true
}

// String writes a pattern as a table cell does: component=value, in the
// components' order.
func (p ownedPattern) String() string {
	var parts []string
	for _, component := range ownedComponents {
		if value, ok := p[component]; ok {
			parts = append(parts, component+"="+value)
		}
	}
	return strings.Join(parts, ", ")
}

// ownedTable renders the transition table as docs/state-machines.md writes
// it, one line per transition.
func ownedTable() []string {
	lines := make([]string, 0, len(ownedTransitions))
	for _, row := range ownedTransitions {
		lines = append(lines, fmt.Sprintf("| %s | %s | %s |", row.event, row.when, row.then))
	}
	sort.Strings(lines)
	return lines
}

// TransitionError is a bug of ours, not a condition of the system: an event
// the transition table has no row for, met in the state it names. A run that
// meets one ends with it as its error, whole, so the command line reports it
// as the internal problem it is.
type TransitionError struct {
	Event string
	State string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("no transition for %q from %s", e.Event, e.State)
}

// ownedLife is one run's state, which only events change. The context's end
// reaches it from os/exec's goroutine, so it is held under a lock. The
// first event the table has no row for is held as the run's failure, and
// every later one answers it, so a run that broke its table still ends its
// cleanup before it returns the error.
type ownedLife struct {
	mu    sync.Mutex
	state ownedState
	table []ownedRow // nil: ownedTransitions
	err   error
}

func newOwnedLife() *ownedLife { return &ownedLife{state: ownedStart} }

func (l *ownedLife) rows() []ownedRow {
	if l.table != nil {
		return l.table
	}
	return ownedTransitions
}

// apply takes the run through an event, or says it has no such transition,
// leaving the state where it was.
func (l *ownedLife) apply(event ownedEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	to, ok := nextIn(l.rows(), l.state, event)
	if !ok {
		l.err = &TransitionError{Event: string(event), State: l.state.String()}
		return l.err
	}
	l.state = to
	return nil
}

// failure is the first event the run met that its table has no row for.
func (l *ownedLife) failure() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

// current is the state the run is in.
func (l *ownedLife) current() ownedState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

// observeContext takes the run through the context's end, if it ended and
// the run has not seen it, by its timeout (ErrCommandTimeout as its cause)
// or its parent, and says how the context stands. Whoever reads the context
// first, os/exec's Cancel or the judgment, takes the step. Both events'
// rows need only context=live, which the step is taken in, so the table
// always has the row and the answer is not asked.
func (l *ownedLife) observeContext(ctx context.Context) ownedContext {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state.context == contextLive && ctx.Err() != nil {
		event := eventContextEndsByParent
		if errors.Is(context.Cause(ctx), ErrCommandTimeout) {
			event = eventContextEndsByTimeout
		}
		l.state, _ = nextIn(l.rows(), l.state, event)
	}
	return l.state.context
}
