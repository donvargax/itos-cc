package main

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
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/config"
	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/mutate"
)

// FreshPreparation is internal evidence collected before any bounded mutant
// trial. Stages describe work actually attempted; a failed preparation is
// never represented as complete coverage.
type FreshPreparation struct {
	Commit     string
	SinceBase  string
	SinceRef   string
	Seed       string
	Algorithm  string
	Units      []mutate.FreshUnit
	Commands   []coverage.CommandExecution
	Stages     []PreparationStage
	Report     *coverage.Report
	Listed     []coverage.Test
	GoCoverage map[string]*mutate.GoCoverageEvidence
	// GoCoverageMissing is each admitted Go function with executable
	// statements that the measurement has no complete inventory of.
	GoCoverageMissing []mutate.FreshUnit
	// Tools is every external executable the measurement and baseline
	// commands need, resolved before any of them runs.
	Tools []PreparationTool
	// Baselines is each own-test command of the selected sites, run once
	// without any mutant, with its time.
	Baselines        []mutate.FreshBaseline
	ExternalBoundary string
	// Tests is the frozen config's mutation.tests, nil when it lists none:
	// how the counted scheduler runs a listed selection.
	Tests *config.Tests
	// Exceptions is the frozen config's mutation.exceptions.
	Exceptions []config.Exception
}

// PreparationTool is one external executable and where it resolved.
type PreparationTool struct {
	Name, Path string
}

type PreparationStage struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// freshPreparationOptions are intentionally internal. No CLI flag or legacy
// complete-run path enables them.
type freshPreparationOptions struct {
	Scope    coverage.Scope
	Executor coverage.CommandExecutor // instance-local test seam; nil uses owned Linux or macOS supervision
	Log      io.Writer
}

// prepareFreshContext measures only the scope frozen by PlanFresh. It admits
// functions from the plan's committed discovery inventory, not from Selected.
func prepareFreshContext(ctx context.Context, plan *mutate.FreshPlan, options freshPreparationOptions) (*FreshPreparation, error) {
	if ctx == nil {
		return nil, errors.New("fresh preparation context must not be nil")
	}
	if plan == nil || plan.FrozenRoot == "" || plan.Commit == "" {
		return nil, errors.New("fresh preparation requires a live frozen plan")
	}
	if len(plan.Units) == 0 {
		return nil, errors.New("fresh preparation has no admitted function scope")
	}
	log := options.Log
	if log == nil {
		log = io.Discard
	}
	prep := &FreshPreparation{Commit: plan.Commit, SinceBase: plan.SinceBase, SinceRef: plan.SinceRef,
		Seed: plan.Seed, Algorithm: plan.Algorithm, Units: append([]mutate.FreshUnit{}, plan.Units...),
		ExternalBoundary: "Tool executables, host environment, Go module cache, and non-Go installed dependencies are outside the frozen commit; missing requirements fail closed and preparation installs nothing. Go toolchain/module/checksum network access is disabled for Go commands."}
	stage := func(name, state string, err error) {
		s := PreparationStage{Name: name, State: state}
		if err != nil {
			s.Error = err.Error()
		}
		prep.Stages = append(prep.Stages, s)
	}
	owned := &mutate.OwnedExecutor{}
	execute := options.Executor
	if execute == nil {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			return prep, errors.New("fresh supervised preparation requires Linux or macOS process-tree ownership; native Windows support is deferred to #29")
		}
		execute = owned.Run
	}
	baseExecute := execute
	gitConfig := ""
	execute = func(ctx context.Context, cmd *exec.Cmd) error {
		cmd.Env = withoutGitOverrides(cmd.Env)
		if gitConfig != "" {
			cmd.Env = withGitConfigEnv(cmd.Env, gitConfig)
		}
		if strings.EqualFold(filepath.Base(cmd.Path), "go") {
			cmd.Env = withEnvOverrides(cmd.Env, map[string]string{
				"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off",
			})
		}
		return baseExecute(ctx, cmd)
	}
	var err error
	gitConfig, err = installFrozenRuntimeGit(ctx, plan, execute, &prep.Commands)
	if err != nil {
		stage("runtime-git", "failed", err)
		return prep, err
	}
	stage("runtime-git", "complete", nil)

	root := plan.FrozenRoot
	cfg, err := config.LoadFrom(root)
	if err != nil {
		stage("admission", "failed", err)
		return prep, fmt.Errorf("read frozen committed config: %w", err)
	}
	prep.Tests, prep.Exceptions = cfg.Tests, cfg.Exceptions
	stage("admission", "complete", nil)
	if err := ctx.Err(); err != nil {
		stage("preparation", "aborted", err)
		return prep, err
	}

	var sources []string
	seen := map[string]bool{}
	for _, unit := range plan.Units {
		p := filepath.Join(root, filepath.FromSlash(unit.Path))
		if !seen[p] {
			seen[p] = true
			sources = append(sources, p)
		}
	}
	private := filepath.Join(root, ".git", "itos-preparation")
	plans, calls, err := coverage.PlansSupervised(ctx, sources, filepath.Join(private, "coverage"), options.Scope, execute)
	prep.Commands = append(prep.Commands, calls...)
	if err != nil {
		stage("coverage-plan", "failed", err)
		return prep, fmt.Errorf("prepare coverage plans: %w", err)
	}
	stage("coverage-plan", "complete", nil)

	baselines, executions, err := mutate.FreshBaselineCommands(ctx, plan, options.Scope == coverage.AllTests, execute)
	prep.Commands = append(prep.Commands, executions...)
	if err != nil {
		stage("baseline-plan", "failed", err)
		return prep, fmt.Errorf("plan frozen baselines: %w", err)
	}
	stage("baseline-plan", "complete", nil)

	tools, err := resolvePreparationTools(plans, baselines, cfg.Tests != nil)
	prep.Tools = tools
	if err != nil {
		stage("tools", "failed", err)
		return prep, err
	}
	stage("tools", "complete", nil)

	var perTest *coverage.PerTest
	if cfg.Tests != nil {
		if err := ctx.Err(); err != nil {
			stage("preparation", "aborted", err)
			return prep, err
		}
		listed, executions, listErr := listFreshTests(ctx, root, cfg.Tests.List, log, execute)
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
		perTest = &coverage.PerTest{Root: root, Tests: listed, Select: cfg.Tests.Select, All: cfg.Tests.All(ids)}
		stage("listed-tests", "complete", nil)
	}
	if err := ctx.Err(); err != nil {
		stage("preparation", "aborted", err)
		return prep, err
	}
	report, executions, coverageErr := coverage.RunSupervised(ctx, plans, sources, log, execute)
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
		listedCoverage, executions, listedErr := coverage.MeasureTestsSupervised(ctx, *perTest,
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
		if options.Scope != coverage.OwnTests && options.Scope != coverage.AllTests {
			err := fmt.Errorf("unsupported strict Go measurement scope %d", options.Scope)
			stage("strict-go-inventory", "failed", err)
			return prep, err
		}
		if !report.Measures("go") {
			err := errors.New("strict Go coverage did not complete a fresh measurement")
			stage("strict-go-inventory", "failed", err)
			return prep, err
		}
		producer := "go test -count=1 -covermode=set -coverprofile=coverage.out; scope=own"
		if options.Scope == coverage.AllTests {
			producer = "go test -count=1 -covermode=set -coverpkg=./... -coverprofile=coverage.out ./...; scope=all-tests"
		}
		var supportPatterns []string
		if cfg.Tests != nil {
			supportPatterns = cfg.Tests.Support
		}
		support, err := mutate.SupportHashes(root, supportPatterns)
		if err != nil {
			stage("strict-go-inventory", "failed", err)
			return prep, fmt.Errorf("fingerprint frozen Go coverage inputs: %w", err)
		}
		prep.GoCoverage = map[string]*mutate.GoCoverageEvidence{}
		for _, unit := range plan.Units {
			if lang.Detect(unit.Path) == nil || !strings.HasSuffix(unit.Path, ".go") {
				continue
			}
			path := filepath.Join(root, filepath.FromSlash(unit.Path))
			unsupported, err := mutate.GoCoverageUnsupported(path)
			if err != nil || unsupported != "" {
				if err == nil {
					err = fmt.Errorf("unsupported Go coverage scope: %s", unsupported)
				}
				stage("strict-go-inventory", "failed", err)
				return prep, err
			}
			inputs, err := mutate.GoCoverageInputs(path, root, producer, support)
			if err != nil {
				stage("strict-go-inventory", "failed", err)
				return prep, err
			}
			inputs["@fresh-preparation-go-network-policy"] = hashPreparationPolicy("GOTOOLCHAIN=local\x00GOPROXY=off\x00GOSUMDB=off")
			evidence, err := mutate.FreshGoCoverageEvidence(root, unit, report.GoBlocks(path), producer, inputs)
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
		ran, executions, baselineErr := mutate.RunFreshBaselines(ctx, baselines, log, execute)
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
func resolvePreparationTools(plans []coverage.Plan, baselines []mutate.FreshBaseline, listed bool) ([]PreparationTool, error) {
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
		if runtime.GOOS == "windows" {
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

func hasGoUnits(units []mutate.FreshUnit) bool {
	for _, unit := range units {
		if lang.Detect(unit.Path) != nil && strings.HasSuffix(unit.Path, ".go") {
			return true
		}
	}
	return false
}

func listFreshTests(ctx context.Context, root, line string, log io.Writer, execute coverage.CommandExecutor) ([]coverage.Test, []coverage.CommandExecution, error) {
	if strings.TrimSpace(line) == "" {
		return nil, nil, errors.New("frozen mutation.tests.list command is empty")
	}
	name, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		name, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(ctx, name, flag, line)
	cmd.Dir = root
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, log
	err := execute(ctx, cmd)
	call := coverage.CommandExecution{Args: append([]string{}, cmd.Args...), Dir: root, Err: err}
	if err != nil {
		return nil, []coverage.CommandExecution{call}, err
	}
	var tests []coverage.Test
	seen := map[string]bool{}
	for _, raw := range strings.Split(output.String(), "\n") {
		id, file, _ := strings.Cut(strings.TrimRight(raw, "\r"), "\t")
		id, file = strings.TrimSpace(id), filepath.ToSlash(strings.TrimSpace(file))
		if id == "" {
			continue
		}
		if strings.ContainsAny(id, "\r\n\t") || seen[id] {
			return nil, []coverage.CommandExecution{call}, fmt.Errorf("malformed or duplicate listed test ID %q", id)
		}
		if file != "" {
			if filepath.IsAbs(file) || file == ".git" || strings.HasPrefix(file, ".git/") || strings.HasPrefix(file, "../") || strings.Contains(file, "\\") {
				return nil, []coverage.CommandExecution{call}, fmt.Errorf("listed test %q names unsupported file %q", id, file)
			}
			clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))
			if clean == ".." || strings.HasPrefix(clean, "../") || clean != file {
				return nil, []coverage.CommandExecution{call}, fmt.Errorf("listed test %q names non-canonical frozen file %q", id, file)
			}
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(file)))
			if err != nil || !info.Mode().IsRegular() {
				return nil, []coverage.CommandExecution{call}, fmt.Errorf("listed test %q file %q is not a committed regular input: %v", id, file, err)
			}
		}
		seen[id] = true
		tests = append(tests, coverage.Test{ID: id, File: file})
	}
	if len(tests) == 0 {
		return nil, []coverage.CommandExecution{call}, errors.New("frozen mutation.tests.list produced no test IDs")
	}
	return tests, []coverage.CommandExecution{call}, nil
}

func installFrozenRuntimeGit(ctx context.Context, plan *mutate.FreshPlan, execute coverage.CommandExecutor, calls *[]coverage.CommandExecution) (string, error) {
	gitDir := filepath.Join(plan.FrozenRoot, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return "", fmt.Errorf("remove isolated ignore-classification metadata: %w", err)
	}
	tempConfig := filepath.Join(plan.FrozenRoot, ".fresh-runtime-gitconfig")
	if err := os.WriteFile(tempConfig, nil, 0o600); err != nil {
		return "", fmt.Errorf("create private Git config: %w", err)
	}
	configDir := filepath.Join(gitDir, "itos-preparation")
	configPath := filepath.Join(configDir, "gitconfig")
	run := func(args ...string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = plan.FrozenRoot
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		activeConfig := configPath
		if args[0] == "init" {
			activeConfig = tempConfig
		}
		cmd.Env = withGitConfigEnv(withoutGitOverrides(nil), activeConfig)
		err := execute(ctx, cmd)
		*calls = append(*calls, coverage.CommandExecution{Args: append([]string{"git"}, args...), Dir: plan.FrozenRoot, Err: err})
		if err != nil {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"fetch", "--quiet", "--no-tags", "--no-recurse-submodules", plan.Root, plan.Commit},
		{"update-ref", "refs/heads/itos-fresh", plan.Commit}, {"symbolic-ref", "HEAD", "refs/heads/itos-fresh"},
		{"read-tree", plan.Commit}} {
		if err := run(args...); err != nil {
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
	*calls = append(*calls, coverage.CommandExecution{Args: []string{"git", "rev-parse", "--verify", "HEAD^{commit}"}, Dir: plan.FrozenRoot, Err: err})
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
	*calls = append(*calls, coverage.CommandExecution{Args: append([]string{"git"}, cmd.Args[1:]...), Dir: plan.FrozenRoot, Err: err})
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
		env = os.Environ()
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
