package architecture_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The graph slice once decided mutation states with a copy of mutation
// check's rule, which fell behind three times (842b8675, 7ac3be89,
// bd2d5119). Its Mutation port is the way back to the one rule, and this
// test proves that nothing else is: a graph package importing the mutation
// slice is refused by depguard and by architecture-check's sibling rule, one
// reading a snapshot of its own through metricsdir or the snapshot store is
// refused by forbidigo, each naming the Mutation port, and one calling the
// port, or only watching the snapshots' folder, passes. The fixtures are
// compile-valid packages in a temporary module, linted by the pinned
// golangci-lint with this repository's .golangci.yml, as TestArchitecture's
// are.
func TestGraphJudgesMutationOnlyThroughItsPort(t *testing.T) {
	root := os.Getenv("ARCHITECTURE_ROOT")
	linter := os.Getenv("ARCHITECTURE_LINTER")
	if root == "" || linter == "" {
		t.Fatal("run tools/bin/architecture-check")
	}
	dir := t.TempDir()
	moduleBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := strings.Fields(string(moduleBytes))[1]
	write := func(path, contents string) {
		t.Helper()
		path = filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module "+module+"\n\ngo 1.27\n")
	// The packages the fixtures import, as small as their callers need.
	write("internal/slices/mutation/domain/stub.go", "package domain\n\ntype Snapshot struct{ Hash string }\n")
	write("internal/snapshot/port/stub.go", "package port\n\nimport \"context\"\n\ntype Store interface {\n\tRead(ctx context.Context, name string, v any) (bool, error)\n\tList(ctx context.Context, dir string) ([]string, error)\n}\n")
	write("internal/metricsdir/stub.go", "package metricsdir\n\nfunc DirOf(root string) string { return root }\n\nfunc ReadIn(dir, name string, v any) (bool, error) { return false, nil }\n\nfunc ListIn(dir, sub string) ([]string, error) { return nil, nil }\n")

	const graph = "internal/slices/graph/"
	// refused names each fixture file a linter must refuse, with the linter
	// and what its message says.
	type refusal struct{ linter, text string }
	refused := map[string]refusal{}
	write(graph+"importdomain/f.go", "package importdomain\n\nimport \"context\"\n\nimport \""+module+"/internal/slices/mutation/domain\"\n\nvar _ = context.Background\n\nfunc Own(s domain.Snapshot) string { return s.Hash }\n")
	refused[graph+"importdomain/f.go"] = refusal{"depguard", "Mutation port"}
	write(graph+"readmetricsdir/f.go", "package readmetricsdir\n\nimport \""+module+"/internal/metricsdir\"\n\nfunc Own(top string) bool {\n\tvar s struct{ Hash string }\n\tfound, err := metricsdir.ReadIn(metricsdir.DirOf(top), \"mutate/a.go.json\", &s)\n\treturn found && err == nil\n}\n\nfunc Names(top string) ([]string, error) { return metricsdir.ListIn(metricsdir.DirOf(top), \"mutate\") }\n")
	refused[graph+"readmetricsdir/f.go"] = refusal{"forbidigo", "Mutation port"}
	write(graph+"readstore/f.go", "package readstore\n\nimport (\n\t\"context\"\n\n\tport \""+module+"/internal/snapshot/port\"\n)\n\nfunc Own(ctx context.Context, store port.Store) bool {\n\tvar s struct{ Hash string }\n\tfound, err := store.Read(ctx, \"mutate/a.go.json\", &s)\n\treturn found && err == nil\n}\n\nfunc Names(ctx context.Context, store port.Store) ([]string, error) { return store.List(ctx, \"mutate\") }\n")
	// Passing: a call of graph's own port, and watching the folder.
	write(graph+"callport/f.go", "package callport\n\nimport \"context\"\n\ntype Mutation interface {\n\tSnapshot(ctx context.Context, top, key string) (any, error)\n\tJudge(ctx context.Context, snapshot any) []bool\n}\n\nfunc Stale(ctx context.Context, m Mutation, top, key string) bool {\n\tsnap, err := m.Snapshot(ctx, top, key)\n\treturn err == nil && len(m.Judge(ctx, snap)) > 0\n}\n")
	write(graph+"watch/f.go", "package watch\n\nimport (\n\t\"os\"\n\n\t\""+module+"/internal/metricsdir\"\n)\n\nfunc Stamp(top string) bool {\n\t_, err := os.Stat(metricsdir.DirOf(top))\n\treturn err == nil\n}\n")

	env := append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	compile := exec.Command("go", "build", "./...")
	compile.Dir = dir
	compile.Env = env
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("fixtures must compile before lint: %v\n%s", err, output)
	}
	outputPath := filepath.Join(dir, "issues.json")
	lint := exec.Command(linter, "run", "--config", filepath.Join(root, ".golangci.yml"), "--output.json.path", outputPath, "--path-mode=abs", "--max-issues-per-linter=0", "--max-same-issues=0", "./...")
	lint.Dir = dir
	lint.Env = env
	output, lintErr := lint.CombinedOutput()
	if lintErr != nil {
		if exit, ok := lintErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("linter failed to run, not a refusal: %v\n%s", lintErr, output)
		}
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read the linter's JSON: %v\n%s", err, output)
	}
	var report struct {
		Issues []struct {
			FromLinter, Text string
			Pos              struct{ Filename string }
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, issue := range report.Issues {
		filename, err := filepath.EvalSymlinks(issue.Pos.Filename)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(canonicalDir, filename)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.ToSlash(rel)
		want, ok := refused[path]
		if !ok || issue.FromLinter != want.linter || !strings.Contains(issue.Text, want.text) {
			t.Errorf("unexpected diagnostic: %s: %s: %s", path, issue.FromLinter, issue.Text)
			continue
		}
		seen[path] = true
	}
	for path, want := range refused {
		t.Run(path, func(t *testing.T) {
			if !seen[path] {
				t.Errorf("missing %s refusal naming %q", want.linter, want.text)
			}
		})
	}
	// The store is an interface forbidigo does not see, so architecture-check
	// reads the names graph hands it: the folder of mutation's snapshots in
	// a string is refused, and the fixtures that call the port or only
	// watch the folder pass.
	check := filepath.Join(root, "tools", "bin", "architecture-check")
	for _, c := range []struct {
		pkg  string
		want bool
	}{{"readstore", true}, {"readmetricsdir", true}, {"callport", false}, {"watch", false}} {
		reads := exec.Command("sh", check, "--graphreads", "internal/slices/graph/"+c.pkg)
		reads.Dir = dir
		reads.Env = env
		out, err := reads.CombinedOutput()
		exit, isExit := err.(*exec.ExitError)
		switch {
		case c.want && (!isExit || exit.ExitCode() != 1 || !strings.Contains(string(out), "Mutation port")):
			t.Errorf("missing the refusal of %s reading a mutation snapshot: %v\n%s", c.pkg, err, out)
		case !c.want && err != nil:
			t.Errorf("refused %s, which reads none: %v\n%s", c.pkg, err, out)
		}
	}
	// The sibling rule refuses the same import with its own message.
	siblings := exec.Command("sh", check, "--siblings", "./internal/slices/graph/importdomain")
	siblings.Dir = dir
	siblings.Env = env
	out, err := siblings.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || !strings.Contains(string(out), "ADR-0051") {
		t.Errorf("missing the sibling rule's refusal of graph importing the mutation slice: %v\n%s", err, out)
	}
}
