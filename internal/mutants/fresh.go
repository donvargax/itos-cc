package mutants

import (
	"context"
	"path/filepath"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	fileselection "github.com/donvargax/itos-cc/internal/selection"
)

// FreshRequest bounds a cache-independent judgment of committed inputs.
type FreshRequest struct {
	Root  string
	Paths []string
	Since string
	Count int
	Seed  string
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

type FreshInvalid struct{ Reason, Key string }

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
