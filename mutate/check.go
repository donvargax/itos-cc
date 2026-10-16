package mutate

import (
	"context"
	"os"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/project"
)

// CoverageInputs is retained for direct legacy coverage callers. Actual command
// checks use the bound domain capabilities, without captured I/O callbacks.
type CoverageInputs func(ctx context.Context, path string) (string, map[string]string, error)

func legacyCheckOptions(store snapport.Store, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception, tools mutants.Toolchain, fingerprints mutants.Fingerprints) mutants.CheckOptions {
	if tools == nil {
		tools = testtools.Toolchain{}
	}
	if fingerprints == nil {
		fingerprints = testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}
	}
	dir, _ := os.Getwd()
	return mutants.CheckOptions{Store: metricsdir.Mutations{Store: store}, Tree: workdir.Tree{}, Surroundings: workdir.Surroundings{}, Root: project.Root(), Dir: dir, Toolchain: tools, Fingerprints: fingerprints, Judge: judge, Tests: tests, Support: support, Exceptions: exceptions}
}

// Check delegates read-only policy to mutants. Direct legacy callers retain
// explicit disk composition until their owning orchestration moves.
func Check(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception) ([]FileCheck, error) {
	return CheckWithInputs(ctx, store, files, judge, tests, support, exceptions, nil, nil)
}
func CheckWithInputs(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception, tools mutants.Toolchain, fingerprints mutants.Fingerprints) ([]FileCheck, error) {
	result, err := mutants.Check(ctx, files, legacyCheckOptions(store, judge, tests, support, exceptions, tools, fingerprints))
	return result, runError(err)
}

// checkParsed is shared with legacy Sample/except/renew. It preserves the
// domain's stale-but-judged outcomes rather than Check's presentation filter.
func checkParsed(ctx context.Context, store snapport.Store, f *lang.File, path string, judge func(path, function, hash string) bool, tests func(path string) []string, support map[string]string, exceptions []config.Exception, tools mutants.Toolchain, fingerprints mutants.Fingerprints) (FileCheck, error) {
	result, err := mutants.CheckParsed(ctx, f, path, legacyCheckOptions(store, judge, tests, support, exceptions, tools, fingerprints))
	return result, runError(err)
}

func CheckGoCoverage(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, current CoverageInputs) ([]CoverageCheck, error) {
	return CheckCoverage(ctx, store, files, judge, map[string]CoverageInputs{"go": current})
}
func CheckCoverage(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, current map[string]CoverageInputs) ([]CoverageCheck, error) {
	return CheckCoverageWithInputs(ctx, store, files, judge, current, nil, nil)
}
func CheckCoverageWithInputs(ctx context.Context, store snapport.Store, files []string, judge func(path, function, hash string) bool, current map[string]CoverageInputs, tools mutants.Toolchain, fingerprints mutants.Fingerprints) ([]CoverageCheck, error) {
	opt := legacyCheckOptions(store, judge, nil, nil, nil, tools, fingerprints)
	opt.Fingerprints = legacyCoverageInputs{Fingerprints: opt.Fingerprints, current: current}
	languages := map[string]bool{}
	for language, observe := range current {
		languages[language] = observe != nil
	}
	result, err := mutants.CheckCoverage(ctx, files, opt, languages, false, false)
	return result, runError(err)
}

// legacyCoverageInputs adapts only the retained callback API. It owns no policy
// and is never composed by actual cmd consumers.
type legacyCoverageInputs struct {
	mutants.Fingerprints
	current map[string]CoverageInputs
}

func (c legacyCoverageInputs) Coverage(ctx context.Context, source, _, _ string, _ map[string]string) (map[string]string, mutants.InputFailure) {
	spec := lang.Detect(source)
	_, hashes, err := c.current[spec.Name](ctx, source)
	if err != nil {
		return nil, mutants.InputFailed{Cause: err}
	}
	return hashes, nil
}
