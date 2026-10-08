package mutate

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/project"
)

// A selection is the set of listed tests a mutant runs once its own tests
// survive it: those whose coverage reaches its line. The first time a
// mutant needs a selection, the selection runs once without any mutant, in
// that mutant's worker's copy, through the same command. Passing proves the
// tests hold alone, and its time sets the timeout of every mutant run of the
// selection, as a file's baseline does for its own tests. A selection that
// fails decides none of the mutants that would run it.

// FailedSelection is a selection of listed tests that fails without any
// mutant.
type FailedSelection struct {
	IDs      []string // in the order the mutants name them
	Command  string   // the shell command line that ran them
	ExitCode int      // the command's exit code, -1 when it gave none
	Output   string
}

// selections holds the baseline of each selection a run needed, by its set
// of IDs, for every worker: each runs once, while the other workers that
// need it wait for it.
type selections struct {
	listed *Listed
	factor float64
	log    io.Writer
	mu     sync.Mutex
	byKey  map[string]*selection
}

// selection is the baseline of one selection: the timeout of its mutant
// runs, or how it failed.
type selection struct {
	once    sync.Once
	timeout time.Duration
	failed  *FailedSelection
	err     error
}

func newSelections(listed *Listed, factor float64, log io.Writer) *selections {
	return &selections{listed: listed, factor: factor, log: log, byKey: map[string]*selection{}}
}

// selectionKey names the set ids, whatever their order.
func selectionKey(ids []string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

// baseline is the baseline of the selection ids, run in w's copy, with no
// mutant, the first time any worker needs it.
func (s *selections) baseline(w *worker, ids []string) *selection {
	key := selectionKey(ids)
	s.mu.Lock()
	sel := s.byKey[key]
	if sel == nil {
		sel = &selection{}
		s.byKey[key] = sel
	}
	s.mu.Unlock()
	sel.once.Do(func() {
		c := s.listed.command(ids)
		fmt.Fprintf(s.log, "itos-cc: baseline %s$ %s\n", project.Rel(c.Dir), c)
		r, err := w.run(c, 0)
		if err != nil {
			sel.err = err
			return
		}
		joined := strings.Join(ids, " ")
		if !r.passed {
			sel.failed = &FailedSelection{IDs: slices.Clone(ids), Command: c.Shell, ExitCode: r.exitCode, Output: r.output}
			fmt.Fprintf(s.log, "itos-cc: baseline of %s failed: no mutant runs it\n", joined)
			return
		}
		sel.timeout = timeoutOf(r.elapsed, s.factor)
		fmt.Fprintf(s.log, "itos-cc: baseline of %s passed in %.1fs: a mutant running it times out after %.1fs\n",
			joined, r.elapsed.Seconds(), sel.timeout.Seconds())
	})
	return sel
}

// timeoutAllowance is added to every mutant's timeout. A mutant's run
// builds the mutated code, which its baseline may have found built, so on
// a slow machine a mutant timed from the baseline alone could time out
// before its tests ran, and a survivor would count as killed. It also
// keeps a fast suite from timing out on scheduling noise.
const timeoutAllowance = 5 * time.Second

// timeoutOf is how long a mutant run may take whose baseline took elapsed:
// factor times it, plus timeoutAllowance.
func timeoutOf(elapsed time.Duration, factor float64) time.Duration {
	return time.Duration(float64(elapsed)*factor) + timeoutAllowance
}

// addFailed records that f, a selection some mutant of the file needed,
// failed without a mutant, once.
func (r *FileResult) addFailed(f *FailedSelection) {
	key := selectionKey(f.IDs)
	if !slices.ContainsFunc(r.FailedSelections, func(g FailedSelection) bool { return selectionKey(g.IDs) == key }) {
		r.FailedSelections = append(r.FailedSelections, *f)
	}
}
