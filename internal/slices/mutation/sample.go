package mutation

import (
	"context"
	"errors"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// SampleQuery carries sample's domain values once: the selection, how many
// mutants to draw, the explicit seed (nil when omitted, so an empty one
// differs from none), the scope given for every mutant and the run's limits.
// Progress is where its observations go as they happen.
type SampleQuery struct {
	SelectionQuery
	Count         int
	Seed          *string
	Workers       int
	TimeoutFactor float64
	OutputFactor  float64
	MemoryFactor  float64
	TestCommand   string
	AllTests      bool
	// Repeat is how many times each sampled mutant runs at least.
	Repeat   int
	Progress SelectionProgress
}

// SelectionProgress receives the observations of sample and run while they
// work: how many sources exclude left out, once the selection is made, and
// each run event.
type SelectionProgress interface {
	mutants.Progress
	Excluded(n int)
}

// SampleFound is what sample observed up to where it stopped: the settled
// seed, whether the selection held no source, and the domain's sample.
type SampleFound struct {
	Seed      string
	NoSources bool
	Sampled   mutants.Sampled
	// NotBuilt are the selected Go files only another system builds,
	// none of whose recorded mutants is drawn.
	NotBuilt []mutants.NotBuilt
}

// errNoWorkers is the one failure after which sample reports no result
// object, as the cmd handler did.
var errNoWorkers = errors.New("mutation Workers binding is missing")

// Sample observes in the cmd handler's order: the selection (the --since
// refusal and range, then exclude), then the seed, which an explicit one
// settles without History, then the no-source return, then settings, the
// tests that reach each source, the listed tests' support files and the
// bindings, then the domain's sample. It writes nothing.
func (h Handler) Sample(ctx context.Context, q SampleQuery) (SampleFound, error) {
	found := SampleFound{}
	sources, judge, _, _, excluded, err := h.selectSources(ctx, q.SelectionQuery)
	if err != nil {
		return found, err
	}
	if q.Progress != nil {
		q.Progress.Excluded(excluded)
	}
	if found.Seed, err = h.seed(ctx, q.Seed); err != nil {
		return found, err
	}
	if len(sources) == 0 {
		found.NoSources = true
		return found, nil
	}
	if sources, found.NotBuilt, err = h.notBuilt(ctx, sources); err != nil || len(sources) == 0 {
		return found, err
	}
	root, cfg, err := h.settings(ctx)
	if err != nil {
		return found, err
	}
	tests, err := h.importingTests(ctx, root)
	if err != nil {
		return found, err
	}
	var support map[string]string
	if cfg.Tests != nil {
		if h.Fingerprints == nil {
			return found, errors.New("mutation Fingerprints binding is missing")
		}
		var failure mutants.InputFailure
		if support, failure = h.Fingerprints.Support(ctx, root, cfg.Tests.Support); failure != nil {
			return found, failure
		}
	}
	if h.Toolchain == nil {
		return found, errors.New("mutation Toolchain binding is missing")
	}
	if h.Fingerprints == nil {
		return found, errors.New("mutation Fingerprints binding is missing")
	}
	if h.Workers == nil {
		return found, errNoWorkers
	}
	opt := mutants.SampleOptions{
		CheckOptions:  h.options(root, judge, tests, support, nil, h.listing(root, cfg)),
		WorkerFactory: h.Workers,
		Workers:       q.Workers,
		TimeoutFactor: q.TimeoutFactor,
		OutputFactor:  q.OutputFactor,
		MemoryFactor:  q.MemoryFactor,
		TestCommand:   q.TestCommand,
		AllTests:      q.AllTests,
		Progress:      q.Progress,
		Repeat:        q.Repeat,
	}
	// An outcome the listed tests decided runs them again, after its own.
	if cfg.Tests != nil && !opt.AllTests && opt.TestCommand == "" {
		opt.Listed = &mutants.Listed{Root: root, Select: cfg.Tests.Select}
	}
	found.Sampled, err = mutants.Sample(ctx, sources, q.Count, found.Seed, opt)
	return found, err
}

// seed is the explicit seed, even an empty one, or the HEAD commit's id.
func (h Handler) seed(ctx context.Context, explicit *string) (string, error) {
	if explicit != nil {
		return *explicit, nil
	}
	if h.History == nil {
		return "", errors.New("mutation history binding is missing")
	}
	id, failure := h.History.Head(ctx)
	if failure != nil {
		return "", SeedUnavailable{Failure: failure}
	}
	return id, nil
}
