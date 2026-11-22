package architecture_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The adapter packages open the selected project's files and run its tools,
// which is what an adapter is for, so .golangci.yml excludes gosec's G304
// (file inclusion by a variable path) and G204 (a subprocess started with a
// variable) there by one path rule, and nowhere else. The fixtures are
// compile-valid packages in a temporary module, linted by the pinned
// golangci-lint with this repository's .golangci.yml, as TestArchitecture's
// are.
func TestGosecPathRule(t *testing.T) {
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
	read := "package probe\n\nimport \"os\"\n\nfunc Read(path string) ([]byte, error) {\n\treturn os.ReadFile(path)\n}\n"
	run := "package probe\n\nimport (\n\t\"os\"\n\t\"os/exec\"\n)\n\nfunc Run() error {\n\ttool := os.Getenv(\"TOOL\")\n\treturn exec.Command(tool).Run()\n}\n"
	both := func(dirPath string) {
		write(dirPath+"/read.go", read)
		write(dirPath+"/run.go", run)
	}
	// refused names each fixture file gosec must refuse, with its rule.
	refused := map[string]string{}
	for _, adapter := range []string{"internal/slices/x/adapters/y", "internal/workdir", "internal/metricsdir", "internal/coveragetools", "internal/project", "internal/processes", "internal/imports"} {
		both(adapter)
	}
	for _, outside := range []string{"internal/covrun", "internal/slices/x", "internal/slices/x/domain", "cmd/probe"} {
		both(outside)
		refused[outside+"/read.go"] = "G304"
		refused[outside+"/run.go"] = "G204"
	}
	// Any other gosec rule stays in an adapter: G306, a file written with
	// permissions looser than 0o600.
	write("internal/slices/x/adapters/y/write.go", "package probe\n\nimport \"os\"\n\nfunc Write(path string) error {\n\treturn os.WriteFile(path, nil, 0o644)\n}\n")
	refused["internal/slices/x/adapters/y/write.go"] = "G306"
	write("internal/workdir/write.go", "package probe\n\nimport \"os\"\n\nfunc Write(path string) error {\n\treturn os.WriteFile(path, nil, 0o644)\n}\n")
	refused["internal/workdir/write.go"] = "G306"

	compile := exec.Command("go", "build", "./...")
	compile.Dir = dir
	compile.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("fixtures must compile before lint: %v\n%s", err, output)
	}
	outputPath := filepath.Join(dir, "issues.json")
	lint := exec.Command(linter, "run", "--config", filepath.Join(root, ".golangci.yml"), "--output.json.path", outputPath, "--path-mode=abs", "--max-issues-per-linter=0", "--max-same-issues=0", "./...")
	lint.Dir = dir
	lint.Env = compile.Env
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
		rule, want := refused[path]
		if !want || issue.FromLinter != "gosec" || !strings.Contains(issue.Text, rule) {
			t.Errorf("unexpected diagnostic: %s: %s: %s", path, issue.FromLinter, issue.Text)
			continue
		}
		seen[path] = true
	}
	for path, rule := range refused {
		t.Run(path, func(t *testing.T) {
			if !seen[path] {
				t.Errorf("missing gosec %s refusal", rule)
			}
		})
	}
}
