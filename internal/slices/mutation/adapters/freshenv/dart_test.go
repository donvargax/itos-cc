package freshenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/covrun"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

const dartFlutterPubspec = "name: board\npublish_to: none\nenvironment:\n  sdk: ^3.6.0\ndependencies:\n  flutter:\n    sdk: flutter\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n"

func TestReadyDartReadiesEachBuildRootOnceInItsDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	in := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	inputs := &porttest.Inputs{Dir: root, Files: map[string]string{
		in("app/pubspec.yaml"): dartFlutterPubspec, in("app/lib/a.dart"): "", in("app/lib/b.dart"): "",
		in("core/pubspec.yaml"): dartFlutterPubspec, in("core/lib/c.dart"): "",
		in("go.mod"): "module m\n", in("main.go"): "package main\n",
	}}
	e := Environment{Toolchain: inputs}
	var ran []string
	execute := func(_ context.Context, cmd *exec.Cmd) error {
		if cmd.Env == nil {
			return fmt.Errorf("%v runs without the preparation environment", cmd.Args)
		}
		ran = append(ran, cmd.Dir+" "+strings.Join(cmd.Args, " "))
		return nil
	}
	var log bytes.Buffer
	calls, err := e.readyDart(t.Context(), []string{in("app/lib/a.dart"), in("main.go"), in("app/lib/b.dart"), in("core/lib/c.dart")}, &log, execute)
	if err != nil {
		t.Fatal(err)
	}
	ready := "flutter --suppress-analytics --no-version-check pub get --offline --enforce-lockfile"
	want := []string{in("app") + " " + ready, in("core") + " " + ready}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("readied %q, want each Flutter package once, offline against its lock: %q", ran, want)
	}
	if len(calls) != 2 || calls[0].Dir != in("app") || strings.Join(calls[1].Args, " ") != ready || calls[1].Err != nil {
		t.Fatalf("recorded %+v, not the two Readies", calls)
	}
	if !strings.Contains(log.String(), "itos-cc: ready "+in("core")+"$ "+ready+"\n") {
		t.Fatalf("the Ready is not announced:\n%s", log.String())
	}
}

func TestReadyDartFailureNamesTheCommandAndItsOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "lib", "a.dart")
	e := Environment{Toolchain: &porttest.Inputs{Dir: root, Files: map[string]string{filepath.Join(root, "pubspec.yaml"): dartFlutterPubspec, source: ""}}}
	execute := func(_ context.Context, cmd *exec.Cmd) error {
		_, _ = io.WriteString(cmd.Stdout, "Unable to satisfy `pubspec.yaml` using `pubspec.lock`.\n")
		return errors.New("exit status 65")
	}
	calls, err := e.readyDart(t.Context(), []string{source}, io.Discard, execute)
	if err == nil || !strings.Contains(err.Error(), "pub get --offline --enforce-lockfile: exit status 65") || !strings.Contains(err.Error(), "Unable to satisfy") {
		t.Fatalf("err = %v, want the failed command and its output", err)
	}
	if len(calls) != 1 || calls[0].Err == nil {
		t.Fatalf("the failed Ready was not recorded: %+v", calls)
	}
}

// A counted run of a Flutter package whose .fvmrc pins an SDK: the live
// .fvm/flutter_sdk is linked into the export, its flutter readies the
// export before any coverage plan, and a failed Ready stops preparation at
// coverage-plan.
func TestFreshPreparationReadiesTheFrozenFlutterPackageWithItsPinnedSDK(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	for name, body := range map[string]string{
		".gitignore": ".dart_tool\n.fvm\n", ".fvmrc": "{\"flutter\": \"stable\"}\n", "pubspec.yaml": dartFlutterPubspec, "pubspec.lock": "packages: {}\n",
		"lib/board.dart": "bool low(int i) {\n  return i == 3;\n}\n", "test/board_test.dart": "import 'package:board/board.dart';\nvoid main() {}\n",
		".fvm/flutter_sdk/bin/flutter": "#!/bin/sh\nexit 1\n",
	} {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "maintenance.auto", "false"}, {"config", "gc.auto", "0"}, {"add", "."},
		{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	plan, err := PlanFresh(t.Context(), repo, nil, "", 1, "ready-test")
	if err != nil {
		t.Fatal(err)
	}
	cleanupFreshPlan(t, plan)
	var others []string
	execute := func(_ context.Context, cmd *exec.Cmd) error {
		switch {
		case commandTool(cmd) == "git":
			return cmd.Run()
		case slices.Contains(cmd.Args, "pub"):
			_, _ = io.WriteString(cmd.Stdout, "Cannot do `--enforce-lockfile` without an existing `pubspec.lock`.\n")
			return errors.New("exit status 65")
		}
		others = append(others, strings.Join(cmd.Args, " "))
		return nil
	}
	prep, err := prepareFreshContext(t.Context(), plan, freshPreparationOptions{Scope: covrun.OwnTests, Executor: execute})
	if err == nil {
		t.Fatalf("a failed Ready passed preparation; stages=%+v", prep.Stages)
	}
	var failed mutants.FreshFailed
	if !errors.As(err, &failed) || failed.Stage != "coverage-plan" {
		t.Fatalf("err = %#v, want preparation failed at coverage-plan", err)
	}
	if last := prep.Stages[len(prep.Stages)-1]; last.Name != "coverage-plan" || last.State != "failed" || !strings.Contains(last.Error, "without an existing `pubspec.lock`") {
		t.Fatalf("last stage = %+v, want coverage-plan failed with the Ready's output", last)
	}
	sdk := filepath.Join(plan.FrozenRoot, ".fvm", "flutter_sdk")
	if target, err := os.Readlink(sdk); err != nil || target != filepath.Join(plan.Root, ".fvm", "flutter_sdk") {
		t.Fatalf("the export's .fvm/flutter_sdk links %q (%v), not the live SDK", target, err)
	}
	readied := prep.Commands[len(prep.Commands)-1]
	want := []string{filepath.Join(sdk, "bin", "flutter"), "--suppress-analytics", "--no-version-check", "pub", "get", "--offline", "--enforce-lockfile"}
	if !reflect.DeepEqual(readied.Args, want) || readied.Dir != plan.FrozenRoot || readied.Err == nil {
		t.Fatalf("the last command = %+v, want the failed Ready of the export with its pinned SDK", readied)
	}
	if len(others) != 0 {
		t.Fatalf("commands ran beside the Ready: %q", others)
	}
}
