// The binary driver's scenario report when scenarios run at once
// (T-128). godog's concurrent runner holds back each scenario's report
// until the scenario ends, then plays it to the formatter one call at a
// time under that scenario's own lock, and prints a feature's header
// directly as its first scenario starts: two scenarios ending together
// interleave their steps, and a scenario can land under another file's
// header. orderedReport takes the pretty report back to the order a run of one
// scenario at a time prints: each feature's header, then its scenarios in
// the file's order, each whole, as soon as it and every scenario before it
// have ended.
package features

import (
	"io"
	"sync"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/formatters"
	messages "github.com/cucumber/messages/go/v34"
)

// orderedFormat is the name TestFeatures gives godog for orderedReport.
const orderedFormat = "itos-cc-ordered"

func init() {
	godog.Format(orderedFormat, "godog's pretty report, each scenario whole and in its file's order, however many run at once",
		func(suite string, out io.Writer) formatters.Formatter {
			return &orderedReport{PrettyFmt: godog.NewPrettyFmt(suite, out), scenarios: map[string]*heldScenario{}}
		})
}

// orderedReport is the pretty formatter fed in the order a serial run feeds it.
// The embedded PrettyFmt keeps godog's storage (SetStorage) and prints;
// orderedReport only holds back and replays what reaches it.
type orderedReport struct {
	*godog.PrettyFmt

	mu        sync.Mutex
	features  []*heldFeature           // the features not printed whole yet, in run order
	scenarios map[string]*heldScenario // each scenario's report, by its pickle's ID
}

// heldFeature is a feature whose scenarios are not all printed: its
// header, whether that is printed, and its scenarios in the file's order,
// those before next printed.
type heldFeature struct {
	document *messages.GherkinDocument
	uri      string
	content  []byte
	printed  bool
	pickles  []*messages.Pickle
	next     int
}

// heldScenario is what reached orderedReport of one scenario: its calls, to
// replay, and how many of its steps have a result. A scenario has ended
// once each of its steps has one, as godog gives every step one, the
// steps after a failure skipped.
type heldScenario struct {
	calls   []func()
	results int
	steps   int
	printed bool
}

// Feature holds the feature's header until its first scenario is printed.
// godog sends it before any of the feature's scenarios start.
func (o *orderedReport) Feature(document *messages.GherkinDocument, uri string, content []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	f := &heldFeature{document: document, uri: uri, content: content}
	if o.Storage != nil {
		f.pickles = o.Storage.MustGetFeature(uri).Pickles
	}
	o.features = append(o.features, f)
	o.drain()
}

func (o *orderedReport) Pickle(pickle *messages.Pickle) {
	o.hold(pickle, false, func() { o.PrettyFmt.Pickle(pickle) })
}

func (o *orderedReport) Defined(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition) {
	o.hold(pickle, false, func() { o.PrettyFmt.Defined(pickle, step, def) })
}

func (o *orderedReport) Passed(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition) {
	o.hold(pickle, true, func() { o.PrettyFmt.Passed(pickle, step, def) })
}

func (o *orderedReport) Skipped(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition) {
	o.hold(pickle, true, func() { o.PrettyFmt.Skipped(pickle, step, def) })
}

func (o *orderedReport) Undefined(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition) {
	o.hold(pickle, true, func() { o.PrettyFmt.Undefined(pickle, step, def) })
}

func (o *orderedReport) Pending(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition) {
	o.hold(pickle, true, func() { o.PrettyFmt.Pending(pickle, step, def) })
}

func (o *orderedReport) Failed(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition, err error) {
	o.hold(pickle, true, func() { o.PrettyFmt.Failed(pickle, step, def, err) })
}

func (o *orderedReport) Ambiguous(pickle *messages.Pickle, step *messages.PickleStep, def *formatters.StepDefinition, err error) {
	o.hold(pickle, true, func() { o.PrettyFmt.Ambiguous(pickle, step, def, err) })
}

// Summary prints whatever is still held, a scenario godog never finished
// among it, so nothing that reached the report is lost, then godog's
// summary of every failed step.
func (o *orderedReport) Summary() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.drain()
	for _, f := range o.features {
		o.header(f)
		for _, p := range f.pickles[f.next:] {
			o.replay(o.scenarios[p.Id])
		}
	}
	o.features = nil
	o.PrettyFmt.Summary()
}

// hold keeps call, a call for pickle's report, a step's result when
// result, and prints what that lets be printed. A call for a scenario
// already printed, which godog does not make, is printed at once rather
// than lost.
func (o *orderedReport) hold(pickle *messages.Pickle, result bool, call func()) {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.scenarios[pickle.Id]
	if s == nil {
		s = &heldScenario{steps: len(pickle.Steps)}
		o.scenarios[pickle.Id] = s
	}
	if s.printed {
		call()
		return
	}
	s.calls = append(s.calls, call)
	if result {
		s.results++
	}
	o.drain()
}

// drain prints, in run order, each feature's header and each scenario that
// has ended, stopping at the first scenario still running.
func (o *orderedReport) drain() {
	for len(o.features) > 0 {
		f := o.features[0]
		for f.next < len(f.pickles) {
			s := o.scenarios[f.pickles[f.next].Id]
			if s == nil || !s.ended() {
				return
			}
			o.header(f)
			o.replay(s)
			f.next++
		}
		o.header(f)
		o.features = o.features[1:]
	}
}

// ended is whether every step of the scenario has its result, or, for a
// scenario of no steps, whether godog has reported it at all.
func (s *heldScenario) ended() bool {
	return s.results >= s.steps && len(s.calls) > 0
}

func (o *orderedReport) header(f *heldFeature) {
	if !f.printed {
		f.printed = true
		o.PrettyFmt.Feature(f.document, f.uri, f.content)
	}
}

func (o *orderedReport) replay(s *heldScenario) {
	if s == nil || s.printed {
		return
	}
	s.printed = true
	for _, call := range s.calls {
		call()
	}
	s.calls = nil
}
