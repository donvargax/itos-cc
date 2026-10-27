package freshenv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/covrun"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

type FreshPreparation = mutants.FreshPreparation
type PreparationStage = mutants.PreparationStage
type PreparationTool = mutants.PreparationTool

// prepareFreshContext measures only the scope frozen by PlanFresh. It admits
// functions from the plan's committed discovery inventory, not from Selected.
func (e Environment) prepare(ctx context.Context, plan *mutants.FreshPlan, scope covrun.Scope) (*FreshPreparation, error) {
	if plan == nil || plan.FrozenRoot == "" || plan.Commit == "" {
		return nil, errors.New("fresh preparation requires a live frozen plan")
	}
	if len(plan.Units) == 0 {
		return nil, errors.New("fresh preparation has no admitted function scope")
	}
	log := e.Log
	if log == nil {
		log = io.Discard
	}
	prep := &FreshPreparation{Commit: plan.Commit, SinceBase: plan.SinceBase, SinceRef: plan.SinceRef,
		Seed: plan.Seed, Algorithm: plan.Algorithm, Units: append([]mutants.FreshUnit{}, plan.Units...)}

	stage := func(name, state string, err error) {
		s := PreparationStage{Name: name, State: state}
		if err != nil {
			s.Error = err.Error()
		}
		prep.Stages = append(prep.Stages, s)
	}
	execute := e.Execute
	if execute == nil {
		if e.Platform != "linux" && e.Platform != "darwin" {
			return prep, errors.New("fresh supervised preparation requires Linux or macOS process-tree ownership; native Windows support is deferred to #29")
		}
		return prep, errors.New("fresh preparation requires an owned executor")
	}
	baseExecute := execute
	gitConfig := ""
	frozen := func(cmd *exec.Cmd) {
		cmd.Env = project.NoBytecodeEnv(withoutGitOverrides(cmd.Env))
		if gitConfig != "" {
			cmd.Env = withGitConfigEnv(cmd.Env, gitConfig)
		}
		if strings.EqualFold(filepath.Base(cmd.Path), "go") {
			cmd.Env = withEnvOverrides(cmd.Env, map[string]string{
				"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off",
			})
		}
	}
	execute = func(ctx context.Context, cmd *exec.Cmd) error {
		frozen(cmd)
		return baseExecute(ctx, cmd)
	}
	// measured runs a baseline as execute does, through Measure, giving
	// what it used, which sizes its mutants' limits; nil without Measure.
	var measured func(context.Context, *exec.Cmd) (mutants.CommandResult, error)
	if e.Measure != nil {
		measured = func(ctx context.Context, cmd *exec.Cmd) (mutants.CommandResult, error) {
			frozen(cmd)
			return e.Measure(ctx, cmd)
		}
	}
	var err error
	gitConfig, err = installFrozenRuntimeGit(ctx, plan, execute, &prep.Commands)
	if err != nil {
		stage("runtime-git", "failed", err)
		return prep, err
	}
	stage("runtime-git", "complete", nil)

	root := plan.FrozenRoot
	cfg, err := settings.LoadFrom(root)
	if err != nil {
		stage("admission", "failed", err)
		return prep, fmt.Errorf("read frozen committed config: %w", err)
	}
	// A plan of its own tests alone lists no test and measures no listed
	// reach, whatever the settings list.
	listedTests := cfg.Tests
	if plan.OwnTestsOnly {
		listedTests = nil
	}
	prep.Tests, prep.Exceptions = listedTests, cfg.Exceptions
	stage("admission", "complete", nil)
	if err := ctx.Err(); err != nil {
		stage("preparation", "aborted", err)
		return prep, err
	}

	sources := mutants.FreshMeasurementSources(plan)
	// The dependencies the live project installed, which the commit never
	// holds, before any plan looks for its tools among them.
	if _, err := linkInstalledDependencies(ctx, plan); err != nil {
		stage("coverage-plan", "failed", err)
		return prep, fmt.Errorf("prepare coverage plans: %w", err)
	}
	// A Flutter package's resolution, which the commit never holds either,
	// before any plan looks for its package config.
	readied, err := e.readyDart(ctx, sources, log, execute)
	prep.Commands = append(prep.Commands, readied...)
	if err != nil {
		stage("coverage-plan", "failed", err)
		return prep, fmt.Errorf("prepare coverage plans: %w", err)
	}
	// A Python or Kotlin file's own tests, and the coverage that decides
	// which of its mutants run, are the tests that reach it in the frozen
	// tree.
	var reach covrun.Reach
	if scope == covrun.OwnTests && hasReachedUnit(plan.Units) {
		tests, err := e.Reach.TestsImporting(ctx, root, plan.Files.Sources, plan.Files.Tests)
		if err != nil {
			stage("coverage-plan", "failed", err)
			return prep, fmt.Errorf("find the tests that reach each frozen file: %w", err)
		}
		reach = func(path string) []string { return tests[path] }
	}
	private := filepath.Join(root, ".git", "itos-preparation")
	plans, calls, err := covrun.PlansSupervised(ctx, sources, filepath.Join(private, "coverage"), scope, reach, execute)
	prep.Commands = append(prep.Commands, calls...)
	if err != nil {
		stage("coverage-plan", "failed", err)
		return prep, fmt.Errorf("prepare coverage plans: %w", err)
	}
	for i := range plans {
		for j := range plans[i].Commands {
			plans[i].Commands[j] = offlineArgs(plans[i].Commands[j])
		}
	}
	stage("coverage-plan", "complete", nil)

	baselines, executions, err := e.baselineCommands(ctx, plan, scope == covrun.AllTests, reach, execute)
	prep.Commands = append(prep.Commands, executions...)
	if err != nil {
		stage("baseline-plan", "failed", err)
		return prep, fmt.Errorf("plan frozen baselines: %w", err)
	}
	// Each trial runs its baseline's command, so it runs offline too.
	for i := range baselines {
		baselines[i].Command.Args = offlineArgs(baselines[i].Command.Args)
	}
	stage("baseline-plan", "complete", nil)

	tools, err := resolvePreparationTools(e.Platform, plans, baselines, listedTests != nil)
	prep.Tools = tools
	if err != nil {
		stage("tools", "failed", err)
		return prep, err
	}
	stage("tools", "complete", nil)

	var perTest *covrun.PerTest
	if listedTests != nil {
		if err := ctx.Err(); err != nil {
			stage("preparation", "aborted", err)
			return prep, err
		}
		listed, executions, listErr := listFreshTests(ctx, e.Platform, root, listedTests.List, log, execute)
		prep.Commands = append(prep.Commands, executions...)
		if listErr != nil {
			stage("listed-tests", "failed", listErr)
			return prep, fmt.Errorf("list frozen mutation tests: %w", listErr)
		}
		prep.Listed = listed
		ids := make([]string, len(listed))
		for i := range listed {
			ids[i] = listed[i].ID
		}
		perTest = &covrun.PerTest{Root: root, Tests: listed, Select: listedTests.Select, All: listedTests.All(ids)}
		stage("listed-tests", "complete", nil)
	}
	if err := ctx.Err(); err != nil {
		stage("preparation", "aborted", err)
		return prep, err
	}
	report, executions, coverageErr := covrun.RunSupervised(ctx, plans, sources, log, execute)
	prep.Commands = append(prep.Commands, executions...)
	prep.Report = report
	if coverageErr != nil {
		stage("coverage", "failed", coverageErr)
		return prep, coverageErr
	}
	stage("coverage", "complete", nil)
	if perTest != nil {
		if err := ctx.Err(); err != nil {
			stage("preparation", "aborted", err)
			return prep, err
		}
		listedCoverage, executions, listedErr := covrun.MeasureTestsSupervised(ctx, *perTest,
			filepath.Join(private, "listed-coverage"), sources, log, execute)
		prep.Commands = append(prep.Commands, executions...)
		if listedErr != nil {
			stage("listed-coverage", "failed", listedErr)
			return prep, fmt.Errorf("measure listed-test reach: %w", listedErr)
		}
		report.SetTests(listedCoverage)
		stage("listed-coverage", "complete", nil)
	}
	if hasGoUnits(plan.Units) {
		if scope != covrun.OwnTests && scope != covrun.AllTests {
			err := fmt.Errorf("unsupported strict Go measurement scope %d", scope)
			stage("strict-go-inventory", "failed", err)
			return prep, err
		}
		if !report.Measures("go") {
			err := errors.New("strict Go coverage did not complete a fresh measurement")
			stage("strict-go-inventory", "failed", err)
			return prep, err
		}
		producer := "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
		if scope == covrun.AllTests {
			producer = "go test -count=1 -covermode=set -coverpkg=./... -coverprofile=coverage.out ./...; scope=all-tests"
		}
		var supportPatterns []string
		if cfg.Tests != nil {
			supportPatterns = cfg.Tests.Support
		}
		support, err := e.Fingerprints.Support(ctx, root, supportPatterns)
		if err != nil {
			stage("strict-go-inventory", "failed", err)
			return prep, fmt.Errorf("fingerprint frozen Go coverage inputs: %w", err)
		}
		prep.GoCoverage = map[string]*mutants.GoCoverageEvidence{}
		for _, unit := range plan.Units {
			if lang.Detect(unit.Path) == nil || !strings.HasSuffix(unit.Path, ".go") {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(unit.Path))
			unsupported, inputFailure := e.Fingerprints.Unsupported(ctx, "go", path, root)
			var err error = inputFailure
			if err != nil || unsupported != "" {
				if err == nil {
					err = fmt.Errorf("unsupported Go coverage scope: %s", unsupported)
				}
				stage("strict-go-inventory", "failed", err)
				return prep, err
			}
			inputs, inputFailure := e.Fingerprints.Coverage(ctx, path, root, producer, support)
			err = inputFailure
			if err != nil {
				stage("strict-go-inventory", "failed", err)
				return prep, err
			}
			inputs["@fresh-preparation-go-network-policy"] = hashPreparationPolicy("GOTOOLCHAIN=local\x00GOPROXY=off\x00GOSUMDB=off")
			evidence, err := e.goCoverageEvidence(ctx, root, unit, report.GoBlocks(path), producer, inputs)
			if err != nil {
				stage("strict-go-inventory", "failed", err)
				return prep, err
			}
			// A function the successful measurement has no complete
			// inventory of, such as one only another OS builds, is a
			// finding about that function, as complete mode reports it,
			// not a failed measurement.
			prep.GoCoverage[unit.Identity] = evidence
			if !evidence.Complete {
				prep.GoCoverageMissing = append(prep.GoCoverageMissing, unit)
			}
		}
		stage("strict-go-inventory", "complete", nil)
	}
	if len(baselines) == 0 {
		stage("baseline", "skipped", nil)
	} else {
		if err := ctx.Err(); err != nil {
			stage("preparation", "aborted", err)
			return prep, err
		}
		ran, executions, baselineErr := runFreshBaselines(ctx, baselines, log, execute, measured)
		prep.Commands = append(prep.Commands, executions...)
		prep.Baselines = ran
		if baselineErr != nil {
			stage("baseline", "failed", baselineErr)
			return prep, baselineErr
		}
		stage("baseline", "complete", nil)
	}
	if err := ctx.Err(); err != nil {
		stage("preparation", "aborted", err)
		return prep, err
	}
	stage("preparation", "complete", nil)
	return prep, nil
}

// resolvePreparationTools resolves, before any of them runs, the executable
// of every coverage and baseline command and the platform shell that listed
// tests run through. A missing one fails preparation: nothing is installed.
func resolvePreparationTools(platform string, plans []covrun.Plan, baselines []mutants.FreshBaseline, listed bool) ([]PreparationTool, error) {
	type need struct{ name, dir string }
	var needs []need
	for _, p := range plans {
		for _, args := range p.Commands {
			if len(args) > 0 {
				needs = append(needs, need{args[0], p.Dir})
			}
		}
	}
	for _, b := range baselines {
		needs = append(needs, need{b.Command.Args[0], b.Command.Dir})
	}
	if listed {
		shell := "sh"
		if platform == "windows" {
			shell = "cmd"
		}
		needs = append(needs, need{shell, ""})
	}
	var tools []PreparationTool
	seen := map[string]bool{}
	var missing []error
	for _, n := range needs {
		name := n.name
		if !filepath.IsAbs(name) && strings.ContainsAny(name, `/\`) {
			// A project-relative tool such as ./gradlew lives where its
			// command runs, not where itos-cc does.
			name = filepath.Join(n.dir, name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		path, err := exec.LookPath(name)
		if err != nil {
			missing = append(missing, fmt.Errorf("required tool %s is not available: %w", n.name, err))
			continue
		}
		tools = append(tools, PreparationTool{Name: n.name, Path: path})
	}
	return tools, errors.Join(missing...)
}

// offlineArgs is args with Gradle told to run --offline and Maven -o, as Go
// commands run with GOPROXY=off: a counted run uses the dependencies their
// local caches hold and downloads none, and one missing offline fails the
// command, and so its preparation stage.
func offlineArgs(args []string) []string {
	if len(args) == 0 || args[0] == "" {
		return args
	}
	name := strings.ToLower(filepath.Base(args[0]))
	for _, ext := range []string{".bat", ".cmd", ".exe"} {
		name = strings.TrimSuffix(name, ext)
	}
	var flag string
	switch name {
	case "gradle", "gradlew":
		flag = "--offline"
	case "mvn", "mvnw":
		flag = "-o"
	default:
		return args
	}
	if slices.Contains(args[1:], flag) {
		return args
	}
	return append([]string{args[0], flag}, args[1:]...)
}

func hasGoUnits(units []mutants.FreshUnit) bool {
	for _, unit := range units {
		if lang.Detect(unit.Path) != nil && strings.HasSuffix(unit.Path, ".go") {
			return true
		}
	}
	return false
}

func listFreshTests(ctx context.Context, platform, root, line string, log io.Writer, execute covrun.CommandExecutor) ([]covrun.Test, []covrun.CommandExecution, error) {
	if strings.TrimSpace(line) == "" {
		return nil, nil, errors.New("frozen mutation.tests.list command is empty")
	}
	name, flag := "sh", "-c"
	if platform == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(ctx, name, flag, line) //nolint:gosec // the committed configuration explicitly supplies this test-list shell command
	cmd.Dir = root
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, log
	err := execute(ctx, cmd)
	call := covrun.CommandExecution{Args: append([]string{}, cmd.Args...), Dir: root, Err: err}
	if err != nil {
		return nil, []covrun.CommandExecution{call}, err
	}
	tests, err := mutants.ParseFreshTests(output.String())
	if err != nil {
		return nil, []covrun.CommandExecution{call}, err
	}
	for _, test := range tests {
		if test.File == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(test.File)))
		if err != nil || !info.Mode().IsRegular() {
			return nil, []covrun.CommandExecution{call}, fmt.Errorf("listed test %q file %q is not a committed regular input: %v", test.ID, test.File, err)
		}
	}
	return tests, []covrun.CommandExecution{call}, nil
}

func installFrozenRuntimeGit(ctx context.Context, plan *mutants.FreshPlan, execute covrun.CommandExecutor, calls *[]covrun.CommandExecution) (string, error) {
	gitDir := filepath.Join(plan.FrozenRoot, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return "", fmt.Errorf("remove isolated ignore-classification metadata: %w", err)
	}
	tempConfig := filepath.Join(plan.FrozenRoot, ".fresh-runtime-gitconfig")
	if err := os.WriteFile(tempConfig, nil, 0o600); err != nil { //nolint:gosec // isolate runtime Git configuration beneath this plan's owned export
		return "", fmt.Errorf("create private Git config: %w", err)
	}
	configDir := filepath.Join(gitDir, "itos-preparation")
	configPath := filepath.Join(configDir, "gitconfig")
	run := func(ctx context.Context, args ...string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // initialize only the private frozen Git repository through the owned executor
		cmd.Dir = plan.FrozenRoot
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		activeConfig := configPath
		if args[0] == "init" {
			activeConfig = tempConfig
		}
		cmd.Env = withGitConfigEnv(withoutGitOverrides(nil), activeConfig)
		err := execute(ctx, cmd)
		*calls = append(*calls, covrun.CommandExecution{Args: append([]string{"git"}, args...), Dir: plan.FrozenRoot, Err: err})
		if err != nil {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"fetch", "--quiet", "--no-tags", "--no-recurse-submodules", plan.Root, plan.Commit},
		{"update-ref", "refs/heads/itos-fresh", plan.Commit}, {"symbolic-ref", "HEAD", "refs/heads/itos-fresh"},
		{"read-tree", plan.Commit}} {
		if err := run(ctx, args...); err != nil {
			return "", err
		}
		if args[0] == "init" {
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				return "", fmt.Errorf("create private Git metadata directory: %w", err)
			}
			if err := os.Rename(tempConfig, configPath); err != nil {
				return "", fmt.Errorf("install private Git config: %w", err)
			}
		}
	}
	for _, scratch := range []string{".fresh-plan-empty-config", ".fresh-plan-empty-excludes", ".fresh-plan-empty-template"} {
		if err := os.RemoveAll(filepath.Join(plan.FrozenRoot, scratch)); err != nil {
			return "", fmt.Errorf("remove planning-only Git scratch %s: %w", scratch, err)
		}
	}
	var head bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD^{commit}")
	cmd.Dir, cmd.Stdout, cmd.Stderr = plan.FrozenRoot, &head, io.Discard
	cmd.Env = withGitConfigEnv(withoutGitOverrides(nil), configPath)
	err := execute(ctx, cmd)
	*calls = append(*calls, covrun.CommandExecution{Args: []string{"git", "rev-parse", "--verify", "HEAD^{commit}"}, Dir: plan.FrozenRoot, Err: err})
	if err != nil {
		return "", fmt.Errorf("verify frozen runtime HEAD: %w", err)
	}
	if strings.TrimSpace(head.String()) != plan.Commit {
		return "", fmt.Errorf("frozen runtime HEAD is %q, want pinned commit %q", strings.TrimSpace(head.String()), plan.Commit)
	}
	var status bytes.Buffer
	cmd = exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir, cmd.Stdout, cmd.Stderr = plan.FrozenRoot, &status, io.Discard
	cmd.Env = withGitConfigEnv(withoutGitOverrides(nil), configPath)
	err = execute(ctx, cmd)
	*calls = append(*calls, covrun.CommandExecution{Args: append([]string{"git"}, cmd.Args[1:]...), Dir: plan.FrozenRoot, Err: err})
	if err != nil {
		return "", fmt.Errorf("verify frozen runtime worktree: %w", err)
	}
	if strings.TrimSpace(status.String()) != "" {
		return "", fmt.Errorf("frozen runtime worktree is not clean: %s", strings.TrimSpace(status.String()))
	}
	return configPath, nil
}

func withoutGitOverrides(env []string) []string {
	if env == nil {
		env = project.InheritedEnv()
	}
	blocked := map[string]bool{
		"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true,
		"GIT_OBJECT_DIRECTORY": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true,
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !blocked[strings.ToUpper(key)] {
			out = append(out, entry)
		}
	}
	return out
}

func withGitConfigEnv(env []string, config string) []string {
	out := make([]string, 0, len(env)+2)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "GIT_CONFIG_") || upper == "GIT_CONFIG_PARAMETERS" {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+config)
}

func withEnvOverrides(env []string, overrides map[string]string) []string {
	out := make([]string, 0, len(env)+len(overrides))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, exists := overrides[strings.ToUpper(key)]; !exists {
			out = append(out, entry)
		}
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

func hashPreparationPolicy(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// hasReachedUnit says whether any admitted function is Python's, Kotlin's
// or Dart's, whose own tests are the tests that reach its file.
func hasReachedUnit(units []mutants.FreshUnit) bool {
	for _, unit := range units {
		if spec := lang.Detect(unit.Path); spec != nil && (spec.Name == "python" || spec.Name == "kotlin" || spec.Name == "dart") {
			return true
		}
	}
	return false
}

// CommandEnvironment gives a trial the environment preparation commands
// get: no parent Git directory, work-tree or index override, and Go
// commands offline with the local toolchain.
func CommandEnvironment(cmd *exec.Cmd) {
	cmd.Env = project.NoBytecodeEnv(withoutGitOverrides(cmd.Env))
	if strings.EqualFold(filepath.Base(cmd.Path), "go") {
		cmd.Env = withEnvOverrides(cmd.Env, map[string]string{"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off"})
	}
}
