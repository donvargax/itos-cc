package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// OwnCoverage is a coverage plan's own tests' coverage a run measured
// (slice-70, issue #67), and what it was measured from: the inputs strict
// coverage evidence records of its build root (Fingerprints.Coverage), and
// the sources it measured. Paths are slash-separated, from the project
// root.
type OwnCoverage struct {
	// Language and Dir, its build root, are the plan's; Tests its own
	// tests, as the plan names them, none for a plan of a build root's.
	Language, Dir string
	Tests         []string
	// Sources are the files the plan measured, sorted.
	Sources []string
	// Inputs are the hashes Fingerprints.Coverage gives of its build root,
	// by their names there.
	Inputs map[string]string
	// Files are the pieces of each source the report names, and Listed
	// the lines it listed, unexecuted, of a source no test loaded (a
	// Kotlin module's report lists them); Proven the sources whose
	// executable lines it lists completely, and Unloaded those no test
	// loaded, both sorted.
	Files, Listed    map[string][]CachedPiece
	Proven, Unloaded []string
}

// PlanKey is what a plan's own coverage rests on, taken before any coverage
// command runs: the entry it would keep, holding no coverage, the name the
// cache keeps it by, and the project root its paths are from.
type PlanKey struct {
	name, root string
	plan       coverage.Plan
	entry      OwnCoverage
}

// The events of a run's use of a plan's cached coverage, naming the plan's
// build root in File: its coverage reused, or the reason, Failure, an entry
// does not serve it.
const (
	OwnCoverageReused   = "own-coverage-reused"
	OwnCoverageUnserved = "own-coverage-unserved"
)

// KeyPlans is the key of each plan m measures req's sources by, measured at
// root (KeyPlanGroups): none when u serves nothing, or req reads reports
// rather than run the built-in commands.
func (u *ListedReuse) KeyPlans(ctx context.Context, m coverage.MutationMeasurer, req covport.Request, tools Toolchain, fp Fingerprints, root string, support map[string]string) []*PlanKey {
	if u == nil || u.Cache == nil || len(req.Reports) > 0 || req.Existing {
		return nil
	}
	return KeyPlanGroups(ctx, tools, fp, root, m.Plans(ctx, req), req.AllTests || req.Scope == covport.AllTests, support)
}

// KeyPlanGroups is the key of each of plans (KeyPlan) whose build root's
// plans of its language all have one: the runner measures them together,
// as it lists an unreached Kotlin plan's lines from its module's report,
// so the cache serves a build root's plans whole or none of them.
func KeyPlanGroups(ctx context.Context, tools Toolchain, fp Fingerprints, root string, plans []coverage.Plan, all bool, support map[string]string) []*PlanKey {
	var keys []*PlanKey
	unkeyed := map[string]bool{}
	for _, plan := range plans {
		key := KeyPlan(ctx, tools, fp, root, plan, all, support)
		if key == nil {
			unkeyed[group(plan)] = true
			continue
		}
		keys = append(keys, key)
	}
	return slices.DeleteFunc(keys, func(k *PlanKey) bool { return unkeyed[group(k.plan)] })
}

// group is plan's language and build root, the plans the runner measures
// together.
func group(plan coverage.Plan) string {
	return plan.Language + "\x00" + plan.Dir
}

// KeyPlan is the key of plan's own coverage, measured at root by its own
// tests or, with all, its build root's whole suite: nil when the cache
// cannot serve it, as its scope is one strict coverage refuses
// (mutation.coverage-unsupported) or an input cannot be read. The name is
// the plan's language, then a hash of its build root from root and, for a
// plan of its own tests, one of them, none for an unreached plan's, so it
// names the plan in any checkout of the project.
func KeyPlan(ctx context.Context, tools Toolchain, fp Fingerprints, root string, plan coverage.Plan, all bool, support map[string]string) *PlanKey {
	if fp == nil || len(plan.Sources) == 0 {
		return nil
	}
	source := plan.Sources[0]
	if unsupported, failure := fp.Unsupported(ctx, plan.Language, source, root); failure != nil || unsupported != "" {
		return nil
	}
	_, inputs, err := CoverageInputsFor(ctx, tools, fp, source, root, all, false, support)
	if err != nil {
		return nil
	}
	key := &PlanKey{root: root, plan: plan,
		entry: OwnCoverage{Language: plan.Language, Dir: snapshot.Key(root, plan.Dir), Tests: slices.Clone(plan.Tests), Inputs: inputs}}
	for _, s := range plan.Sources {
		key.entry.Sources = append(key.entry.Sources, snapshot.Key(root, s))
	}
	slices.Sort(key.entry.Sources)
	key.name = plan.Language + "-" + shortHash(key.entry.Dir)
	if plan.Tests != nil || plan.Unreached {
		key.name += "-" + shortHash(strings.Join(plan.Tests, "\x00"))
	}
	return key
}

// shortHash is the first eight hex digits of text's SHA-256.
func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:4])
}

// ServePlans is the coverage u serves of the plans of keys, the sources of
// the plans it served, and the keys of those it did not. It serves a plan
// when the entry kept of it was measured from what its key says, of the
// same sources, its paths then at the key's root, and every plan of its
// build root and language among keys is served too (KeyPlanGroups); it
// says which it served, and why an entry there does not serve the run.
func (u *ListedReuse) ServePlans(ctx context.Context, keys []*PlanKey) (served []*coverage.Report, sources []string, rest []*PlanKey) {
	reports := make([]*coverage.Report, len(keys))
	unserved := map[string]bool{}
	for i, key := range keys {
		if reports[i] = u.lookup(ctx, key); reports[i] == nil {
			unserved[group(key.plan)] = true
		}
	}
	for i, key := range keys {
		if unserved[group(key.plan)] {
			rest = append(rest, key)
			continue
		}
		report(u.Progress, RunEvent{Kind: OwnCoverageReused, File: key.plan.Dir})
		served = append(served, reports[i])
		sources = append(sources, key.plan.Sources...)
	}
	return served, sources, rest
}

// lookup is the coverage of key's plan the entry kept of it holds, at the
// key's root, when it was measured from what key says, of the same
// sources; nil otherwise, having said why an entry there does not serve
// the run. A nil reuse serves nothing.
func (u *ListedReuse) lookup(ctx context.Context, key *PlanKey) *coverage.Report {
	if u == nil || u.Cache == nil {
		return nil
	}
	entry, failure := u.Cache.Own(ctx, key.name)
	if _, cancelled := failure.(*snapport.Cancelled); cancelled {
		return nil
	}
	if failure != nil {
		report(u.Progress, RunEvent{Kind: OwnCoverageUnserved, File: key.plan.Dir, Failure: ListedUnserved{Unreadable: true}})
		return nil
	}
	if entry == nil {
		return nil
	}
	if why := key.entry.unserved(*entry); why != nil {
		report(u.Progress, RunEvent{Kind: OwnCoverageUnserved, File: key.plan.Dir, Failure: *why})
		return nil
	}
	return coverage.FromPlan(entry.Language, key.plan.Dir, key.atRoot(entry.Files), key.atRoot(entry.Listed), key.paths(entry.Proven), key.paths(entry.Unloaded))
}

// unserved is why kept, an entry of the plan k names, does not serve a run
// k keys, nil when it serves it: the inputs that differ, else the run's
// sources it did not measure, else those it measured besides them.
func (k OwnCoverage) unserved(kept OwnCoverage) *ListedUnserved {
	var why ListedUnserved
	for _, name := range differing(k.Inputs, kept.Inputs) {
		why.Changed = append(why.Changed, strings.TrimPrefix(name, "support:"))
	}
	for _, file := range k.Sources {
		if !slices.Contains(kept.Sources, file) {
			why.Unmeasured = append(why.Unmeasured, file)
		}
	}
	for _, file := range kept.Sources {
		if !slices.Contains(k.Sources, file) {
			why.Also = append(why.Also, file)
		}
	}
	if len(why.Changed)+len(why.Unmeasured)+len(why.Also) == 0 {
		return nil
	}
	slices.Sort(why.Changed)
	return &why
}

// KeepPlans keeps what report measured of each plan of keys, when the run
// writes and its built-in measurement succeeded whole, as Keep keeps the
// listed tests' coverage: each in place of the entry kept of its plan. A
// cache it cannot write is left as it is.
func (u *ListedReuse) KeepPlans(ctx context.Context, keys []*PlanKey, report *coverage.Report) {
	if u == nil || !u.Write || u.Cache == nil ||
		len(report.Missing()) > 0 || len(report.Failed()) > 0 || len(report.IntegrationMissing()) > 0 {
		return
	}
	for _, key := range keys {
		entry := key.entry
		entry.Files, entry.Listed = map[string][]CachedPiece{}, map[string][]CachedPiece{}
		unloaded := report.Unloaded(key.plan.Language)
		for _, source := range key.plan.Sources {
			rel := snapshot.Key(key.root, source)
			if report.Has(source) {
				entry.Files[rel] = report.Pieces(source)
			}
			if listed := report.Listed(source); listed != nil {
				entry.Listed[rel] = listed
			}
			if report.Proves(source) {
				entry.Proven = append(entry.Proven, rel)
			}
			if slices.Contains(unloaded, source) {
				entry.Unloaded = append(entry.Unloaded, rel)
			}
		}
		slices.Sort(entry.Proven)
		slices.Sort(entry.Unloaded)
		_ = u.Cache.SaveOwn(ctx, key.name, entry)
	}
}

// atRoot is files, by paths the cache keeps, by their paths at the key's
// root.
func (k *PlanKey) atRoot(files map[string][]CachedPiece) map[string][]coverage.Piece {
	out := map[string][]coverage.Piece{}
	for file, pieces := range files {
		out[atRoot(k.root, file)] = pieces
	}
	return out
}

// paths is each of files, paths the cache keeps, at the key's root.
func (k *PlanKey) paths(files []string) []string {
	var out []string
	for _, file := range files {
		out = append(out, atRoot(k.root, file))
	}
	return out
}
