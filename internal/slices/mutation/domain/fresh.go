package domain

import (
	"context"
	"path/filepath"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	fileselection "github.com/donvargax/itos-cc/internal/selection"
)

// FreshRequest bounds a cache-independent judgment of committed inputs:
// Count sites drawn with Seed from Paths and Since's changed functions, or,
// with Sites, the mutants they name alone (AdmitNamed). OwnTestsOnly
// judges them by their own tests alone (FreshPlan.OwnTestsOnly).
type FreshRequest struct {
	Root         string
	Paths        []string
	Since        string
	Count        int
	Seed         string
	Sites        []FreshSite
	OwnTestsOnly bool
}

// FreshPlan is a cache-independent selection of committed mutation sites.
// FrozenRoot holds the repository's committed tracked inputs only; external
// tools, dependencies and environment are outside this freeze. FrozenRoot is
// private scratch space owned by Fresh and removed by Release.
type FreshPlan struct {
	Root       string
	FrozenRoot string
	Commit     string
	SinceRef   string // caller's --since spelling
	SinceBase  string // once-resolved base commit used for every range diff
	Seed       string
	Algorithm  string
	Eligible   []FreshCandidate
	Selected   []FreshCandidate
	Omitted    []FreshCandidate
	// Units is the admitted committed function scope, including functions with
	// no mutation sites. Preparation must measure this scope without rebuilding
	// it from Selected, which contains sites only.
	Units []FreshUnit
	// Files is every committed source and test file of the frozen tree,
	// selected or not, as discovery classifies them: what the tests that
	// reach a file are found among.
	Files fileselection.Files
	// Changed holds, with --since, the lines of each changed committed
	// source, by its root-relative slash path, that the range added or
	// changed: the new side of each hunk, renamed files under their new
	// path, and none for a pure deletion. Nil without --since.
	Changed map[string][]LineRange
	// Uncommitted is every root-relative slash path, sorted, whose live
	// version differs from Commit when the plan was made: staged, unstaged,
	// or untracked and not ignored, a deletion included. The run judges
	// none of these changes (UncommittedInputs).
	Uncommitted []string
	// NotBuilt are the committed Go sources only another system builds,
	// by their root-relative slash paths: no unit or site of theirs is
	// admitted (AdmitFresh).
	NotBuilt []NotBuilt
	// Named is, for a plan of named sites (AdmitNamed), each argument with
	// the identities it named; nil for a drawn plan.
	Named []FreshNamed
	// OwnTestsOnly is whether the request judges the selected mutants by
	// their own tests alone (mutation run --site --own-tests): preparation
	// then lists no test and measures no listed reach, whatever the
	// settings list, and leaves its Tests nil, so no trial runs one.
	OwnTestsOnly bool
	// ListedReuse, when set, serves the listed tests' coverage preparation
	// measures from the cache of the live project, the plan's Root, which
	// it never writes (MUT-185): preparation keys the frozen tree's inputs
	// (KeyListed) and measures only what the cache does not serve.
	ListedReuse *ListedReuse
}

// FreshSettings is the settings file a counted run reads at its commit, at
// the repository's top level: config.File, which the domain does not
// import.
const FreshSettings = "itos-cc.yaml"

// UncommittedInputs are the paths of Uncommitted that are inputs the run
// froze at Commit, and so judged as committed, not as they are now: the
// sources and tests of a supported language, and the settings. A change
// to anything else, a README say, is no input of the judgment. Never nil.
func (p *FreshPlan) UncommittedInputs() []string {
	out := []string{}
	for _, rel := range p.Uncommitted {
		if rel == FreshSettings || lang.Detect(rel) != nil {
			out = append(out, rel)
		}
	}
	return out
}

// ChangedLines reports whether the --since range added or changed any line
// from start to end, both included, of the committed source rel.
func (p *FreshPlan) ChangedLines(rel string, start, end int) bool {
	return OverlapsRanges(p.Changed[filepath.ToSlash(rel)], start, end)
}

// FreshBaseline is one clean own-test run in a plan's frozen root.
type FreshBaseline struct {
	Command Command
	Paths   []string
	Elapsed time.Duration
	// Output is the bytes its run wrote and Memory the most memory its
	// process group held, where MemoryMeasured says the system measured
	// it: with Elapsed, what sizes its mutants' limits (Usage).
	Output, Memory int64
	MemoryMeasured bool
}

// Usage is what the baseline's run used.
func (b FreshBaseline) Usage() Usage {
	return Usage{Elapsed: b.Elapsed, Output: b.Output, Memory: b.Memory, MemoryMeasured: b.MemoryMeasured}
}

// FreshPreparation is actual ordered evidence, including work completed before
// failure. No persistent mutation or reusable coverage result is published.
type FreshPreparation struct {
	Commit, SinceBase, SinceRef, Seed, Algorithm string
	Units                                        []FreshUnit
	Commands                                     []coverage.CommandExecution
	Stages                                       []PreparationStage
	Report                                       *coverage.Report
	Listed                                       []coverage.Test
	GoCoverage                                   map[string]*GoCoverageEvidence
	GoCoverageMissing                            []FreshUnit
	Tools                                        []PreparationTool
	Baselines                                    []FreshBaseline
	Tests                                        *Tests
	Exceptions                                   []Exception
}

type PreparationTool struct{ Name, Path string }
type PreparationStage struct{ Name, State, Error string }

// FreshFailure preserves preparation/refusal facts without presentation policy.
//
//sumtype:decl
type FreshFailure interface {
	error
	freshFailure()
}

type FreshRefusal string

const (
	FreshRepository  FreshRefusal = "repository"
	FreshSince       FreshRefusal = "since"
	FreshPath        FreshRefusal = "path"
	FreshScratch     FreshRefusal = "scratch"
	FreshUnsupported FreshRefusal = "unsupported"
)

type FreshRefused struct {
	Kind  FreshRefusal
	Cause error
}

func (FreshRefused) freshFailure()   {}
func (f FreshRefused) Error() string { return f.Cause.Error() }
func (f FreshRefused) Unwrap() error { return f.Cause }

// FreshInvalid is settings the plan cannot read, Reason saying why, Key
// naming the key at fault where one is, and Fix how to mend them where the
// refusal has its own mending.
type FreshInvalid struct{ Reason, Key, Fix string }

func (FreshInvalid) freshFailure()   {}
func (f FreshInvalid) Error() string { return f.Reason }

type FreshFailed struct {
	Stage string
	Cause error
}

func (FreshFailed) freshFailure()   {}
func (f FreshFailed) Error() string { return f.Cause.Error() }
func (f FreshFailed) Unwrap() error { return f.Cause }

type FreshCancelled struct{ Cause error }

func (FreshCancelled) freshFailure()   {}
func (f FreshCancelled) Error() string { return f.Cause.Error() }
func (f FreshCancelled) Unwrap() error { return f.Cause }

// Fresh prepares an immutable committed environment and retains it
// through all joined trials. Release preserves physical best-effort cleanup.
type Fresh interface {
	Plan(ctx context.Context, request FreshRequest) (*FreshPlan, FreshFailure)
	Prepare(ctx context.Context, plan *FreshPlan, scope covport.Scope) (*FreshPreparation, FreshFailure)
	Release(ctx context.Context, plan *FreshPlan)
}
