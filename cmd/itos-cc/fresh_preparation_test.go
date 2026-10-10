package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/mutate"
)

// These tests exercise new internal preparation capabilities, not behavioral
// reds against the original product. They run no mutant trials.

func TestFreshPreparationReportsEveryCommandFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	if err := os.WriteFile(source, []byte("package source\nfunc Value() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(root, "lcov.info")
	writeReport := func() {
		t.Helper()
		data := fmt.Sprintf("TN:\nSF:%s\nDA:2,1\nend_of_record\n", filepath.ToSlash(source))
		if err := os.WriteFile(report, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := coverage.Plan{Language: "typescript", Dir: root, Sources: []string{source},
		Commands: [][]string{{"coverage-helper", "first"}, {"coverage-helper", "second"}}, Reports: []string{report}}
	var attempted []string
	execute := func(_ context.Context, cmd *exec.Cmd) error {
		attempted = append(attempted, strings.Join(cmd.Args, " "))
		writeReport() // A usable artifact does not disguise the nonzero exit.
		return fmt.Errorf("helper exited nonzero")
	}
	measured, calls, err := coverage.RunSupervised(context.Background(), []coverage.Plan{plan}, []string{source}, os.Stderr, execute)
	if err == nil {
		t.Fatal("supervised preparation accepted failed commands that wrote a report")
	}
	if len(attempted) != 2 || len(calls) != 2 || calls[0].Err == nil || calls[1].Err == nil {
		t.Fatalf("commands not completely captured: attempted=%v calls=%+v", attempted, calls)
	}
	if measured == nil || !measured.Has(source) {
		t.Fatalf("usable partial report should remain visible alongside the failure: %#v", measured)
	}
}

func TestFreshPreparationOwnsCommandsAndPreservesListedReach(t *testing.T) {
	if runtime.GOOS == "linux" {
		testOwnedPreparationCleanup(t)
	}
	repo := freshPreparationFixture(t)
	baseCmd := exec.Command("git", "rev-parse", "HEAD")
	baseCmd.Dir = repo
	base, err := baseCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc Value(x int) int { return x + 2 }\nfunc Empty() {\n// still no executable statements\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "main.go"}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "range source change"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit range fixture with git %v: %v\n%s", args, err, out)
		}
	}
	plan, err := mutate.PlanFresh(repo, nil, strings.TrimSpace(string(base)), 1, "prep-test")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if len(plan.Units) < 2 {
		t.Fatalf("frozen scope omitted zero-site function: %+v", plan.Units)
	}
	changed := filepath.Join(repo, "main.go")
	if err := os.WriteFile(changed, []byte("package main\nfunc Value(x int) int { return x * 99 }\nfunc Empty() {\n// changed live function\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "main.go"}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "advance branch"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("advance original branch with git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "late.go"), []byte("package main\nfunc Late() int { return 9 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("package main\nfunc Value(x int) int { return x - 777 }\nfunc Empty() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repo, "itos-cc.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(strings.ReplaceAll(string(configData), "fresh-list", "live-list")), 0o600); err != nil {
		t.Fatal(err)
	}
	stageConfig := exec.Command("git", "add", "itos-cc.yaml")
	stageConfig.Dir = repo
	if out, err := stageConfig.CombinedOutput(); err != nil {
		t.Fatalf("stage live config edit: %v\n%s", err, out)
	}
	current := exec.Command("git", "rev-parse", "HEAD")
	current.Dir = repo
	currentHead, err := current.Output()
	if err != nil || strings.TrimSpace(string(currentHead)) == plan.Commit {
		t.Fatalf("fixture did not advance original branch: %s (%v)", currentHead, err)
	}
	// Parent Git overrides, as a hook sets them, must not reach the private
	// runtime repository or the repositories tests create.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-a-repository"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
	var calls []string
	execute := freshPreparationExecutor(t, &calls, nil)
	prep, err := prepareFreshContext(context.Background(), plan, freshPreparationOptions{Scope: coverage.OwnTests, Executor: execute})
	if err != nil {
		t.Fatalf("prepare fresh fixture: %v; stages=%+v calls=%v", err, prep.Stages, calls)
	}
	if prep.Commit != plan.Commit || prep.SinceBase != strings.TrimSpace(string(base)) || prep.SinceBase != plan.SinceBase || prep.SinceRef != strings.TrimSpace(string(base)) || prep.Seed != plan.Seed {
		t.Fatalf("preparation changed frozen provenance: plan=%+v prep=%+v", plan, prep)
	}
	if len(prep.Listed) != 1 || prep.Listed[0].ID != "T-1" || len(prep.GoCoverage) != len(plan.Units) {
		t.Fatalf("listed or strict zero-site scope was lost: listed=%+v evidence=%+v units=%+v", prep.Listed, prep.GoCoverage, plan.Units)
	}
	if got := prep.Report.LineTests(filepath.Join(plan.FrozenRoot, "main.go"), 2); len(got) != 1 || got[0] != "T-1" {
		t.Fatalf("listed test reach = %v, want [T-1]", got)
	}
	if len(calls) < 8 {
		t.Fatalf("expected runtime Git, Go planning, listing, coverage and conversion commands; saw %v", calls)
	}
	if len(prep.Baselines) != 1 || !slices.Equal(prep.Baselines[0].Paths, []string{"main.go"}) || prep.Baselines[0].Command.Dir != plan.FrozenRoot {
		t.Fatalf("baselines = %+v, want one own-test run of main.go in the frozen root", prep.Baselines)
	}
	if len(prep.Commands) != len(calls) {
		t.Fatalf("recorded %d command outcomes for %d commands run: %+v", len(prep.Commands), len(calls), prep.Commands)
	}
	for _, call := range prep.Commands {
		if call.Err != nil {
			t.Fatalf("a successful preparation recorded a failed command: %+v", call)
		}
	}
	var tools []string
	for _, tool := range prep.Tools {
		if tool.Path == "" {
			t.Fatalf("tool %s recorded without its resolved path", tool.Name)
		}
		tools = append(tools, tool.Name)
	}
	if !slices.Contains(tools, "go") {
		t.Fatalf("required tools %v do not name go", tools)
	}
	var stages []string
	for _, stage := range prep.Stages {
		if stage.State != "complete" {
			t.Fatalf("stage %+v of a successful preparation is not complete", stage)
		}
		stages = append(stages, stage.Name)
	}
	want := []string{"runtime-git", "admission", "coverage-plan", "baseline-plan", "tools", "listed-tests", "coverage", "listed-coverage", "strict-go-inventory", "baseline", "preparation"}
	if !slices.Equal(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
}

func TestFreshPreparationFailsClosedOnBaselineFailure(t *testing.T) {
	repo := freshPreparationFixture(t)
	plan, err := mutate.PlanFresh(repo, nil, "", 1, "prep-test")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	var calls []string
	execute := freshPreparationExecutor(t, &calls, func(cmd *exec.Cmd) error {
		if commandTool(cmd) == "go" && slices.Contains(cmd.Args, "-failfast") {
			return errors.New("own tests fail without any mutant")
		}
		return nil
	})
	prep, err := prepareFreshContext(context.Background(), plan, freshPreparationOptions{Scope: coverage.OwnTests, Executor: execute})
	if err == nil {
		t.Fatalf("a failing baseline passed preparation; stages=%+v", prep.Stages)
	}
	last := prep.Stages[len(prep.Stages)-1]
	if last.Name != "baseline" || last.State != "failed" || last.Error == "" {
		t.Fatalf("last stage = %+v, want the failed baseline", last)
	}
	failing := prep.Commands[len(prep.Commands)-1]
	if failing.Err == nil || !slices.Contains(failing.Args, "-failfast") {
		t.Fatalf("the failing baseline was not recorded as failed: %+v", failing)
	}
	if len(prep.Baselines) != 0 {
		t.Fatalf("a failed baseline was kept as a timing: %+v", prep.Baselines)
	}
}

func TestFreshPreparationRefusesMissingTools(t *testing.T) {
	plans := []coverage.Plan{{Language: "go", Dir: t.TempDir(), Commands: [][]string{{"itos-cc-t12-no-such-tool", "test"}}}}
	tools, err := resolvePreparationTools(plans, nil, false)
	if err == nil || !strings.Contains(err.Error(), "itos-cc-t12-no-such-tool") {
		t.Fatalf("missing tool was not refused: tools=%+v err=%v", tools, err)
	}
}

func TestFreshPreparationScopesGoPackagesFromWholeListing(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":           "module example.com/scope\n\ngo 1.22\n",
		"main.go":          "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n",
		"unused/unused.go": "package unused\n\nfunc Dormant() int { return 2 }\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sources := []string{filepath.Join(root, "main.go"), filepath.Join(root, "unused", "unused.go")}
	listing := func(text string, fail error) coverage.CommandExecutor {
		return func(_ context.Context, cmd *exec.Cmd) error {
			if commandTool(cmd) != "go" || cmd.Args[1] != "list" {
				return fmt.Errorf("unexpected planning command %v", cmd.Args)
			}
			if fail != nil {
				return fail
			}
			_, err := io.WriteString(cmd.Stdout, text)
			return err
		}
	}
	// go list prints a package without dependencies last, its line ending in
	// the tab before its empty dependency list.
	whole := fmt.Sprintf("example.com/scope\t%s\tfmt\nexample.com/scope/unused\t%s\t\n", root, filepath.Join(root, "unused"))
	plans, calls, err := coverage.PlansSupervised(context.Background(), sources, t.TempDir(), coverage.OwnTests, listing(whole, nil))
	if err != nil || len(plans) != 1 || len(calls) != 1 || calls[0].Err != nil {
		t.Fatalf("supervised Go plan: plans=%+v calls=%+v err=%v", plans, calls, err)
	}
	if args := plans[0].Commands[0]; !slices.Contains(args, "example.com/scope/unused") || !slices.Contains(args, "example.com/scope") {
		t.Fatalf("supervised own-test command %v does not measure every admitted package", args)
	}
	for name, run := range map[string]coverage.CommandExecutor{
		"failed":    listing("", errors.New("go list exited 1")),
		"empty":     listing("", nil),
		"malformed": listing(fmt.Sprintf("example.com/scope\t%s\n", root), nil),
		"omitted":   listing(fmt.Sprintf("example.com/scope\t%s\tfmt\n", root), nil),
	} {
		plans, calls, err := coverage.PlansSupervised(context.Background(), sources, t.TempDir(), coverage.OwnTests, run)
		if err == nil {
			t.Errorf("%s go list was accepted: plans=%+v", name, plans)
		}
		if len(calls) != 1 || (name == "failed") != (calls[0].Err != nil) {
			t.Errorf("%s go list outcome not recorded as run: %+v", name, calls)
		}
	}
}

// freshPreparationExecutor stands in for every preparation command: real Git
// for the private runtime repository, and small controlled helpers for Go,
// listing and listed coverage. fail, when set, may refuse a command first.
func freshPreparationExecutor(t *testing.T, calls *[]string, fail func(*exec.Cmd) error) coverage.CommandExecutor {
	t.Helper()
	return func(ctx context.Context, cmd *exec.Cmd) error {
		*calls = append(*calls, filepath.Base(cmd.Path)+" "+strings.Join(cmd.Args[1:], " "))
		if cmd.Env == nil {
			return fmt.Errorf("preparation command inherits the parent environment unfiltered: %v", cmd.Args)
		}
		for _, entry := range cmd.Env {
			key, _, _ := strings.Cut(entry, "=")
			switch strings.ToUpper(key) {
			case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE":
				return fmt.Errorf("parent Git override %s leaked into %v", entry, cmd.Args)
			}
		}
		if fail != nil {
			if err := fail(cmd); err != nil {
				return err
			}
		}
		if commandTool(cmd) == "go" && len(cmd.Args) > 1 && cmd.Args[1] == "test" && coverageFlag(cmd.Args, "-coverprofile=") == "" {
			// The unmutated baseline of the selected sites' own tests.
			if !slices.Contains(cmd.Args, "-failfast") {
				return fmt.Errorf("baseline is not the mutant command: %v", cmd.Args)
			}
			return nil
		}
		if commandTool(cmd) == "git" {
			return cmd.Run()
		}
		if commandTool(cmd) == "go" && len(cmd.Args) > 1 && cmd.Args[1] == "list" {
			frozen, err := os.ReadFile(filepath.Join(cmd.Dir, "main.go"))
			if err != nil || strings.Contains(string(frozen), "99") || strings.Contains(string(frozen), "777") {
				return fmt.Errorf("execution did not retain pinned committed source: %s (%v)", frozen, err)
			}
			frozenConfig, err := os.ReadFile(filepath.Join(cmd.Dir, "itos-cc.yaml"))
			if err != nil || strings.Contains(string(frozenConfig), "live-list") {
				return fmt.Errorf("execution did not retain pinned committed config: %s (%v)", frozenConfig, err)
			}
			if _, err := os.Stat(filepath.Join(cmd.Dir, "late.go")); !os.IsNotExist(err) {
				return fmt.Errorf("untracked live source leaked into frozen execution: %v", err)
			}
			fmt.Fprintf(cmd.Stdout, "example.com/prep\t%s\t\n", cmd.Dir)
			return nil
		}
		if commandTool(cmd) == "go" && len(cmd.Args) > 1 && cmd.Args[1] == "test" {
			profile := coverageFlag(cmd.Args, "-coverprofile=")
			return writeGoProfile(profile, filepath.Join(cmd.Dir, "main.go"))
		}
		if commandTool(cmd) == "go" && len(cmd.Args) > 3 && cmd.Args[1] == "tool" && cmd.Args[2] == "covdata" {
			return writeGoProfile(coverageFlag(cmd.Args, "-o="), filepath.Join(cmd.Dir, "main.go"))
		}
		if len(cmd.Args) >= 3 && (cmd.Args[2] == "fresh-list" || cmd.Args[2] == "live-list" || cmd.Args[2] == "fresh-listed-all" || strings.HasPrefix(cmd.Args[2], "fresh-listed-select ")) {
			switch {
			case cmd.Args[2] == "fresh-list":
				_, err := fmt.Fprintf(cmd.Stdout, "T-1\tmain_test.go\n")
				return err
			case cmd.Args[2] == "live-list":
				return errors.New("live staged config leaked into frozen preparation")
			default:
				dir := ""
				for _, entry := range cmd.Env {
					if strings.HasPrefix(entry, coverage.TestCoverDirEnv+"=") {
						dir = strings.TrimPrefix(entry, coverage.TestCoverDirEnv+"=")
					}
				}
				if dir == "" {
					for _, entry := range cmd.Env {
						if strings.HasPrefix(entry, "GOCOVERDIR=") {
							dir = strings.TrimPrefix(entry, "GOCOVERDIR=")
						}
					}
				}
				if err := os.MkdirAll(filepath.Join(dir, "T-1"), 0o700); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "T-1", "covmeta.fixture"), nil, 0o600)
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("unexpected preparation command: %s %v", cmd.Path, cmd.Args)
	}
}

func commandTool(cmd *exec.Cmd) string {
	name := strings.ToLower(filepath.Base(cmd.Path))
	return strings.TrimSuffix(name, ".exe")
}

func testOwnedPreparationCleanup(t *testing.T) {
	t.Helper()
	owned := &mutate.OwnedExecutor{}
	sentinel := exec.Command("sleep", "30")
	if err := sentinel.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = sentinel.Process.Kill()
		_, _ = sentinel.Process.Wait()
	}()

	// A normally completed parent must not leave an ordinary descendant
	// holding its output pipe, and must not signal an unrelated process.
	marker := filepath.Join(t.TempDir(), "descendant-after-return")
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sh -c 'sleep 0.2; touch \"$1\"' sh \"$1\" & exit 0", "sh", marker)
	if err := owned.Run(context.Background(), cmd); err != nil {
		t.Fatalf("owned normal completion: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("owned descendant continued fixture work after return: %v", err)
	}
	if err := sentinel.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("owned cleanup affected unrelated sentinel: %v", err)
	}

	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	cmd = exec.CommandContext(ctx, "sh", "-c", "sleep 30 & echo $! > \"$1\"; wait", "sh", pidFile)
	go func() { finished <- owned.Run(ctx, cmd) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("owned helper did not publish descendant PID")
		}
		time.Sleep(5 * time.Millisecond)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("parent cancellation was reported as successful preparation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owned parent/descendant did not finish cleanup")
	}
	if runningProcess(pid) {
		t.Fatalf("owned descendant %d remained runnable after cancellation", pid)
	}
	if err := sentinel.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("cancellation cleanup affected unrelated sentinel: %v", err)
	}
}

func runningProcess(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	closeParen := strings.LastIndexByte(string(data), ')')
	if closeParen < 0 {
		return true
	}
	fields := strings.Fields(string(data[closeParen+1:]))
	return len(fields) > 0 && fields[0] != "Z" && fields[0] != "X"
}

func TestFreshPreparationPreservesCompleteDefaults(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	if err := os.WriteFile(source, []byte("package source\nfunc Value() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(root, "coverage.out")
	plan := coverage.Plan{Language: "go", Dir: root, Sources: []string{source},
		Commands: [][]string{{"legacy-helper"}}, Reports: []string{report}}
	legacy := coverage.Run([]coverage.Plan{plan}, []string{source}, os.Stderr)
	if legacy == nil || len(legacy.Missing()) == 0 {
		t.Fatalf("legacy complete-run soft failure behavior changed: %#v", legacy)
	}
	if _, _, err := coverage.RunSupervised(context.Background(), []coverage.Plan{plan}, []string{source}, os.Stderr,
		func(context.Context, *exec.Cmd) error { return errors.New("owned helper failed") }); err == nil {
		t.Fatal("opt-in preparation must fail closed while legacy Run remains soft")
	}
}

func freshPreparationFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":       "module example.com/prep\n\ngo 1.22\n",
		"main.go":      "package main\nfunc Value(x int) int { return x + 1 }\nfunc Empty() {}\n",
		"main_test.go": "package main\nimport \"testing\"\nfunc TestValue(t *testing.T) { _ = Value(1) }\n",
		"itos-cc.yaml": "mutation:\n  tests:\n    list: fresh-list\n    run: fresh-listed-select {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n    whole: fresh-listed-all\n",
	}
	for name, body := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}, {"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func coverageFlag(args []string, prefix string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func writeGoProfile(path, source string) error {
	if path == "" {
		return errors.New("missing output profile path")
	}
	data := fmt.Sprintf("mode: set\n%s:2.1,2.40 1 1\n", filepath.ToSlash(source))
	return os.WriteFile(path, []byte(data), 0o600)
}
