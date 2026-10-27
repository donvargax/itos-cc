package porttest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/lang"
	sel "github.com/donvargax/itos-cc/internal/selection"
	selt "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Fresh owns independent immutable Git state and actual virtual frozen bytes.
// DecodeConfig is a pure composition seam to the one settings codec. It reads
// only supplied committed bytes, never the live tree or an expected outcome.
type Fresh struct {
	History      History
	Coverage     *covtest.Measurer
	Commands     *Workers
	Lister       mutants.TestLister
	Tools        []mutants.PreparationTool
	DecodeConfig func([]byte) ([]string, *mutants.Tests, []mutants.Exception, *mutants.FreshInvalid)
	Inputs       *Inputs
	Tree         *selt.Tree
	// Measured, when set, observes each collection preparation completes
	// in a plan's frozen inputs: its own tests', or the listed tests' when
	// listed is not nil, as their harness observes the runs it makes.
	Measured func(ctx context.Context, p *mutants.FreshPlan, listed *coverage.PerTest) error
	// Log, when set, is where preparation says what it runs, as the real
	// preparation says it on itos-cc's stderr: each own-tests baseline's
	// command, before it runs.
	Log      io.Writer
	sequence uint64
	plans    map[*mutants.FreshPlan]bool
}

var _ mutants.Fresh = (*Fresh)(nil)

func (f *Fresh) Plan(ctx context.Context, r mutants.FreshRequest) (*mutants.FreshPlan, mutants.FreshFailure) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.FreshCancelled{Cause: cause}
	}
	if f.DecodeConfig == nil || f.Inputs == nil || f.Tree == nil {
		return nil, mutants.FreshFailed{Cause: fmt.Errorf("fresh planning bindings are missing")}
	}
	if r.Count <= 0 && len(r.Sites) == 0 {
		return nil, mutants.FreshFailed{Cause: fmt.Errorf("fresh plan count must be positive")}
	}
	requested := r.Root
	if requested == "" {
		requested = f.Inputs.Dir
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(f.Inputs.Dir, requested)
	}
	requested = filepath.Clean(requested)
	if !inside(f.History.Root, requested) || !f.Inputs.Exists(ctx, requested) {
		return nil, mutants.FreshRefused{Kind: mutants.FreshRepository, Cause: fmt.Errorf("no Git repository with a resolvable HEAD: %s", requested)}
	}
	if _, file := f.Inputs.Files[requested]; file {
		return nil, mutants.FreshRefused{Kind: mutants.FreshRepository, Cause: fmt.Errorf("not a repository directory: %s", requested)}
	}
	head, failed := f.History.Head(ctx)
	if failed != nil {
		return nil, mutants.FreshRefused{Kind: mutants.FreshRepository, Cause: failed}
	}
	paths, err := mutants.CanonicalFreshPaths(r.Paths)
	if err != nil {
		return nil, err.(mutants.FreshFailure)
	}
	committed := f.History.Repository.Commits[head].Files
	// Only regular committed files are exported: a committed symbolic link
	// is refused rather than followed or materialized.
	if links := slices.Sorted(maps.Keys(f.History.Repository.Commits[head].Links)); len(links) > 0 {
		return nil, mutants.FreshRefused{Kind: mutants.FreshUnsupported, Cause: fmt.Errorf("unsupported committed scope: committed symlink %q", links[0])}
	}
	var changed map[string][]mutants.LineRange
	base := ""
	if r.Since != "" {
		base, failed = f.History.Resolve(ctx, r.Since)
		if failed != nil {
			return nil, mutants.FreshRefused{Kind: mutants.FreshSince, Cause: failed}
		}
		common := f.History.mergeBase(base, head)
		if common == "" {
			return nil, mutants.FreshFailed{Cause: fmt.Errorf("git diff %s...%s: no merge base", base, head)}
		}
		before := f.History.Repository.Commits[common].Files
		moves := historyRenames(before, committed)
		prior := map[string]string{}
		for old, current := range moves {
			prior[current] = old
		}
		changed = map[string][]mutants.LineRange{}
		for name, text := range committed {
			old := name
			if prior[name] != "" {
				old = prior[name]
			}
			if previous, ok := before[old]; !ok || previous != text || old != name {
				changed[name] = historyRanges(before[old], text)
			}
		}
	}
	var configBytes []byte
	if text, ok := committed["itos-cc.yaml"]; ok {
		configBytes = []byte(text)
	}
	exclude, _, _, invalid := f.DecodeConfig(configBytes)
	if invalid != nil {
		return nil, *invalid
	}
	for chosen := range paths {
		found := false
		for name := range committed {
			if mutants.SelectsFreshPath(name, map[string]bool{chosen: true}) && supportedSource(name) {
				found = true
				break
			}
		}
		if !found {
			return nil, mutants.FreshRefused{Kind: mutants.FreshPath, Cause: fmt.Errorf("bad fresh plan path: %q selects no committed supported source", chosen)}
		}
	}
	f.sequence++
	root := f.History.Root
	frozen := filepath.Join(root, ".git", "itos", "fresh-plan-memory-"+strconv.FormatUint(f.sequence, 10))
	local := maps.Clone(committed)
	tree := selt.TreeOf(frozen, local, nil)
	around := &Inputs{Dir: frozen, Files: map[string]string{}, Runtime: f.Inputs.Runtime, Probes: f.Inputs.Probes}
	for name, text := range local {
		around.Files[filepath.Join(frozen, filepath.FromSlash(name))] = text
	}
	sources, tests, problem := tree.Discover(ctx, []string{frozen})
	if problem != nil {
		return nil, mutants.FreshFailed{Cause: problem}
	}
	all := sel.Files{Sources: sources, Tests: tests}
	if len(paths) > 0 {
		roots := []string{}
		for name := range paths {
			roots = append(roots, filepath.Join(frozen, filepath.FromSlash(name)))
		}
		slices.Sort(roots)
		sources, _, problem = tree.Discover(ctx, roots)
		if problem != nil {
			return nil, mutants.FreshFailed{Cause: problem}
		}
	}
	seed := r.Seed
	if seed == "" {
		seed = head
	}
	plan := &mutants.FreshPlan{Root: root, FrozenRoot: frozen, Commit: head, SinceRef: r.Since, SinceBase: base, Seed: seed, Algorithm: mutants.FreshPlanVersion, Files: all,
		Uncommitted: f.History.uncommitted(committed)}
	if changed != nil {
		plan.Changed = map[string][]mutants.LineRange{}
		for name, ranges := range changed {
			for _, r := range ranges {
				if !r.Deletion {
					plan.Changed[name] = append(plan.Changed[name], r)
				}
			}
		}
	}
	names := slices.Sorted(maps.Keys(committed))
	if len(r.Sites) > 0 {
		err = mutants.AdmitNamed(ctx, plan, tree, around, around, sources, names, sel.Exclusion{Root: frozen, Exclude: exclude}, r.Sites)
	} else {
		err = mutants.AdmitFresh(ctx, plan, tree, around, around, sources, names, sel.Exclusion{Root: frozen, Exclude: exclude}, changed, r.Count)
	}
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return nil, mutants.FreshCancelled{Cause: cause}
		}
		var unmatched mutants.FreshUnmatched
		if errors.As(err, &unmatched) {
			return nil, unmatched
		}
		return nil, mutants.FreshFailed{Cause: err}
	}
	// Publish only actual completed materialization. Subsequent preparation/trials
	// observe this shared memory, not the plan-time parse or the mutable live files.
	if f.Inputs.Files == nil {
		f.Inputs.Files = map[string]string{}
	}
	for name, text := range around.Files {
		f.Inputs.Files[name] = text
	}
	// One tree can serve live and several owned frozen plans through relative keys.
	if f.Tree.Contents == nil {
		f.Tree.Contents = map[string]string{}
	}
	for name, text := range around.Files {
		rel, _ := filepath.Rel(f.Tree.Dir, name)
		f.Tree.Contents[filepath.ToSlash(rel)] = text
	}
	for _, name := range tree.Sources {
		abs := filepath.Join(frozen, filepath.FromSlash(name))
		rel, _ := filepath.Rel(f.Tree.Dir, abs)
		f.Tree.Sources = append(f.Tree.Sources, filepath.ToSlash(rel))
	}
	for _, name := range tree.Tests {
		abs := filepath.Join(frozen, filepath.FromSlash(name))
		rel, _ := filepath.Rel(f.Tree.Dir, abs)
		f.Tree.Tests = append(f.Tree.Tests, filepath.ToSlash(rel))
	}
	if f.plans == nil {
		f.plans = map[*mutants.FreshPlan]bool{}
	}
	f.plans[plan] = true
	if f.Commands != nil {
		if f.Commands.Originals == nil {
			f.Commands.Originals = map[string]string{}
		}
		f.Commands.Originals[filepath.Clean(frozen)] = filepath.Clean(root)
	}
	f.projectObservations(plan)
	return plan, nil
}
func (f *Fresh) Release(_ context.Context, p *mutants.FreshPlan) {
	if p == nil || !f.plans[p] {
		return
	}
	root := p.FrozenRoot
	f.releaseObservations(root)
	if f.Commands != nil {
		delete(f.Commands.Originals, filepath.Clean(root))
	}
	for name := range f.Inputs.Files {
		if inside(root, name) {
			delete(f.Inputs.Files, name)
		}
	}
	for name := range f.Tree.Contents {
		if inside(root, filepath.Join(f.Tree.Dir, filepath.FromSlash(name))) {
			delete(f.Tree.Contents, name)
		}
	}
	f.Tree.Sources = slices.DeleteFunc(f.Tree.Sources, func(name string) bool { return inside(root, filepath.Join(f.Tree.Dir, filepath.FromSlash(name))) })
	f.Tree.Tests = slices.DeleteFunc(f.Tree.Tests, func(name string) bool { return inside(root, filepath.Join(f.Tree.Dir, filepath.FromSlash(name))) })
	p.FrozenRoot = ""
	delete(f.plans, p)
}
func supportedSource(path string) bool { return lang.Detect(path) != nil }

// Prepare is implemented from actual frozen settings and collector/worker facts,
// not a declaration of expected stages or final mutation outcomes.
func (f *Fresh) Prepare(ctx context.Context, p *mutants.FreshPlan, scope covport.Scope) (*mutants.FreshPreparation, mutants.FreshFailure) {
	return f.prepare(ctx, p, scope)
}
