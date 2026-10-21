package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func (f *Fresh) prepare(ctx context.Context, p *mutants.FreshPlan, scope covport.Scope) (*mutants.FreshPreparation, mutants.FreshFailure) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, mutants.FreshCancelled{Cause: cause}
	}
	if p == nil || p.FrozenRoot == "" || !f.plans[p] {
		return nil, mutants.FreshFailed{Cause: fmt.Errorf("fresh preparation requires a live frozen plan")}
	}
	if len(p.Units) == 0 {
		return nil, mutants.FreshFailed{Cause: fmt.Errorf("fresh preparation has no admitted function scope")}
	}
	prep := &mutants.FreshPreparation{Commit: p.Commit, SinceBase: p.SinceBase, SinceRef: p.SinceRef, Seed: p.Seed, Algorithm: p.Algorithm, Units: slices.Clone(p.Units)}
	stage := func(name, state string, err error) {
		s := mutants.PreparationStage{Name: name, State: state}
		if err != nil {
			s.Error = err.Error()
		}
		prep.Stages = append(prep.Stages, s)
	}
	stop := func(name string, err error) (*mutants.FreshPreparation, mutants.FreshFailure) {
		state := "failed"
		if cause := context.Cause(ctx); cause != nil {
			state = "aborted"
			stage(name, state, cause)
			return prep, mutants.FreshCancelled{Cause: cause}
		}
		stage(name, state, err)
		return prep, mutants.FreshFailed{Stage: name, Cause: err}
	}
	// No physical Git command is claimed here: the independent immutable fixture
	// commit and frozen virtual tree provide the runtime identity directly.
	stage("runtime-git", "complete", nil)
	if f.DecodeConfig == nil {
		return stop("admission", errors.New("fresh config decoder is missing"))
	}
	text, exists := f.Inputs.Files[filepath.Join(p.FrozenRoot, "itos-cc.yaml")]
	var data []byte
	if exists {
		data = []byte(text)
	}
	_, tests, exceptions, invalid := f.DecodeConfig(data)
	if invalid != nil {
		stage("admission", "failed", *invalid)
		return prep, *invalid
	}
	prep.Tests, prep.Exceptions = tests, exceptions
	stage("admission", "complete", nil)
	sources := mutants.FreshMeasurementSources(p)
	inputs := *f.Inputs
	inputs.Dir = p.FrozenRoot
	if f.Inputs.Imports != nil {
		inputs.Imports = map[string][]string{}
		for path, edges := range f.Inputs.Imports {
			if inside(p.FrozenRoot, path) {
				inputs.Imports[path] = slices.Clone(edges)
			}
		}
	}
	var reach map[string][]string
	if scope == covport.OwnTests {
		var failure mutants.InputFailure
		reach, failure = inputs.TestsImporting(ctx, p.FrozenRoot, p.Files.Sources, p.Files.Tests)
		if failure != nil {
			return stop("coverage-plan", failure)
		}
	}
	stage("coverage-plan", "complete", nil)
	var baselines []mutants.FreshBaseline
	for _, rel := range mutants.FreshSelectedPaths(p) {
		path := filepath.Join(p.FrozenRoot, filepath.FromSlash(rel))
		request := mutants.CommandRequest{Path: path, All: scope == covport.AllTests, Tests: reach[path]}
		observer := inputs
		if spec := lang.Detect(path); spec != nil && spec.Name == "python" {
			observer.Dir = observer.base(ctx, path, "pyproject.toml", "setup.py", "setup.cfg")
		}
		facts, failed := observer.Observe(ctx, request)
		if failed != nil {
			return stop("baseline-plan", failed)
		}
		command := mutants.ChooseCommand(request, facts)
		if facts.Language == "python" && len(command.Args) > 0 {
			var err error
			if !facts.Pytest {
				err = errors.New("independent pytest import failed")
			}
			prep.Commands = append(prep.Commands, coverage.CommandExecution{Args: []string{facts.Interpreter, "-c", "import pytest"}, Dir: facts.Root, Err: err})
		}
		var err error
		baselines, err = mutants.AddFreshBaseline(baselines, p.FrozenRoot, rel, command)
		if err != nil {
			return stop("baseline-plan", err)
		}
	}
	stage("baseline-plan", "complete", nil)
	// Tool presence is declared independently of command outcomes. Missing command
	// facts are not converted to an invented installation or successful baseline.
	if f.Coverage == nil || f.Coverage.Declarations == nil {
		return stop("tools", errors.New("fresh collector observations are missing"))
	}
	for _, b := range baselines {
		wanted := b.Command.Args[0]
		found := false
		for _, tool := range f.Tools {
			if tool.Name == wanted {
				found = true
				prep.Tools = append(prep.Tools, tool)
				break
			}
		}
		if !found {
			return stop("tools", fmt.Errorf("required tool %s is not available", wanted))
		}
	}
	stage("tools", "complete", nil)
	var perTest *coverage.PerTest
	if tests != nil {
		if f.Lister == nil {
			return stop("listed-tests", errors.New("fresh test listing binding is missing"))
		}
		listing, failed := f.Lister.List(ctx, mutants.Command{Root: p.FrozenRoot, Dir: p.FrozenRoot, Shell: tests.List})
		if failed != nil {
			return stop("listed-tests", failed)
		}
		name, flag := "sh", "-c"
		if inputs.Runtime.Platform == "windows" {
			name, flag = "cmd", "/C"
		}
		var listErr error
		if listing.ExitCode != 0 {
			listErr = errors.New(listing.Reason)
		}
		prep.Commands = append(prep.Commands, coverage.CommandExecution{Args: []string{name, flag, tests.List}, Dir: p.FrozenRoot, Err: listErr})
		if listErr != nil {
			return stop("listed-tests", listErr)
		}
		listed, parseErr := mutants.ParseFreshTests(listing.Stdout)
		if parseErr != nil {
			return stop("listed-tests", parseErr)
		}
		for _, test := range listed {
			if test.File != "" {
				if _, ok := f.Inputs.Files[filepath.Join(p.FrozenRoot, filepath.FromSlash(test.File))]; !ok {
					return stop("listed-tests", fmt.Errorf("listed test %q file %q is not a committed regular input", test.ID, test.File))
				}
			}
		}
		prep.Listed = listed
		ids := []string{}
		for _, test := range listed {
			ids = append(ids, test.ID)
		}
		perTest = &coverage.PerTest{Root: p.FrozenRoot, Tests: listed, Select: tests.Select, All: tests.All(ids)}
		stage("listed-tests", "complete", nil)
	}
	req := covport.Request{Sources: sources, Scope: scope, AllTests: scope == covport.AllTests, Reach: func(path string) []string { return reach[path] }}
	// The parser reads actual independently declared tool-format artifacts. A
	// failed command can retain those bytes but cannot erase the stage failure.
	report, err := f.Coverage.CollectFresh(ctx, req)
	prep.Report = report
	if err != nil {
		return stop("coverage", err)
	}
	if f.Measured != nil {
		if err := f.Measured(ctx, p, nil); err != nil {
			return stop("coverage", err)
		}
	}
	stage("coverage", "complete", nil)
	if perTest != nil {
		tc, err := f.Coverage.MeasureFreshTests(ctx, req, perTest)
		if err != nil {
			return stop("listed-coverage", err)
		}
		if f.Measured != nil {
			if err := f.Measured(ctx, p, perTest); err != nil {
				return stop("listed-coverage", err)
			}
		}
		report.SetTests(tc)
		stage("listed-coverage", "complete", nil)
	}
	goUnits := false
	for _, u := range p.Units {
		if strings.HasSuffix(u.Path, ".go") {
			goUnits = true
		}
	}
	if goUnits {
		if scope != covport.OwnTests && scope != covport.AllTests {
			return stop("strict-go-inventory", fmt.Errorf("unsupported strict Go measurement scope %d", scope))
		}
		if !report.Measures("go") {
			return stop("strict-go-inventory", errors.New("strict Go coverage did not complete a fresh measurement"))
		}
		producer := mutants.CoverageProducer("go", scope == covport.AllTests)
		var supportPatterns []string
		if tests != nil {
			supportPatterns = tests.Support
		}
		support, failed := inputs.Support(ctx, p.FrozenRoot, supportPatterns)
		if failed != nil {
			return stop("strict-go-inventory", failed)
		}
		prep.GoCoverage = map[string]*mutants.GoCoverageEvidence{}
		for _, u := range p.Units {
			if !strings.HasSuffix(u.Path, ".go") {
				continue
			}
			path := filepath.Join(p.FrozenRoot, filepath.FromSlash(u.Path))
			unsupported, failed := inputs.Unsupported(ctx, "go", path, p.FrozenRoot)
			if failed != nil {
				return stop("strict-go-inventory", failed)
			}
			if unsupported != "" {
				return stop("strict-go-inventory", fmt.Errorf("unsupported Go coverage scope: %s", unsupported))
			}
			hashes, failed := inputs.Coverage(ctx, path, p.FrozenRoot, producer, support)
			if failed != nil {
				return stop("strict-go-inventory", failed)
			}
			policy := sha256.Sum256([]byte("GOTOOLCHAIN=local\x00GOPROXY=off\x00GOSUMDB=off"))
			hashes["@fresh-preparation-go-network-policy"] = hex.EncodeToString(policy[:])
			file, err := selection.Parse(ctx, *f.Tree, &inputs, path)
			if err != nil {
				return stop("strict-go-inventory", err)
			}
			evidence, err := mutants.FreshGoCoverageEvidence(file, u, report.GoBlocks(path), producer, hashes)
			file.Close()
			if err != nil {
				return stop("strict-go-inventory", err)
			}
			prep.GoCoverage[u.Identity] = evidence
			if !evidence.Complete {
				prep.GoCoverageMissing = append(prep.GoCoverageMissing, u)
			}
		}
		stage("strict-go-inventory", "complete", nil)
	}
	if len(baselines) == 0 {
		stage("baseline", "skipped", nil)
	} else {
		if f.Commands == nil {
			return stop("baseline", errors.New("fresh baseline command observations are missing"))
		}
		for _, b := range baselines {
			if cause := context.Cause(ctx); cause != nil {
				return stop("baseline", cause)
			}
			c := b.Command
			f.bindBaseline(p, c)
			facts, known := f.Commands.Tests[filepath.Clean(c.Dir)][c.String()]
			if !known {
				return stop("baseline", fmt.Errorf("no tests declared for %s$ %s", c.Dir, c.String()))
			}
			if facts.Trace != nil {
				if err := f.Commands.observeTrace(c, maps.Clone(f.Inputs.Files), facts.Trace, c.Dir, f.Commands.original(c.Root), harnessFields(facts, nil)); err != nil {
					return stop("baseline", err)
				}
			}
			actual, failed := declaredResult(c, facts, nil, 0)
			var err error = failed
			if failed == nil && !actual.Passed {
				err = fmt.Errorf("command exited %d: %s", actual.ExitCode, actual.Output)
			}
			prep.Commands = append(prep.Commands, coverage.CommandExecution{Args: slices.Clone(c.Args), Dir: c.Dir, Err: err})
			if err != nil {
				return stop("baseline", fmt.Errorf("baseline of %s fails without any mutant: %w", strings.Join(b.Paths, ", "), err))
			}
			b.Elapsed = actual.Elapsed
			prep.Baselines = append(prep.Baselines, b)
		}
		stage("baseline", "complete", nil)
	}
	stage("preparation", "complete", nil)
	return prep, nil
}
