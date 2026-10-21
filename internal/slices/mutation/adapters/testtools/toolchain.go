// Package testtools observes installed mutation test runners and runnable tests.
// Pure command policy belongs to mutants; optional absence retains its fallback.
package testtools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/project"
)

// Toolchain observes the real project. Dir is the actual caller cwd for the
// interpreter probe, independently of the eventual test Command.Root/Dir.
// Empty Dir retains the process cwd for direct legacy library callers.
type Toolchain struct {
	Dir string
	Env []string
}

func (t Toolchain) Observe(ctx context.Context, r mutants.CommandRequest) (mutants.ToolFacts, mutants.InputFailure) {
	var f mutants.ToolFacts
	if err := context.Cause(ctx); err != nil {
		return f, mutants.InputCancelled{Cause: err}
	}
	spec := lang.Detect(r.Path)
	if spec == nil {
		return f, nil
	}
	f.Language = spec.Name
	switch spec.Name {
	case "dart":
		// Its package, each pub workspace member a build root of its own,
		// run by the flutter coverage runs it with, FVM's preferred.
		f.Root = Base(ctx, r.Path, "pubspec.yaml")
		flutter := coverage.FlutterFor(ctx, f.Root)
		if flutter == "" {
			// No flutter is found: the run names the one it lacks.
			flutter = "flutter"
		}
		f.Flutter = append([]string{flutter}, coverage.FlutterQuiet...)
		if !r.All {
			f.DartFiles = coverage.DartTests(f.Root, r.Tests)
		}
	case "go":
		f.Root = Base(ctx, r.Path, "go.mod")
	case "typescript":
		f.Root = Base(ctx, r.Path, "package.json")
		deps := packageDeps(f.Root)
		if deps["vitest"] {
			f.Vitest = project.NodeBin(f.Root, "vitest")
		}
		if deps["jest"] {
			f.Jest = project.NodeBin(f.Root, "jest")
		}
		if f.Vitest == "" && f.Jest == "" {
			f.PackageManager = project.PackageManager(f.Root)
		}
	case "python":
		f.Root = Base(ctx, r.Path, "pyproject.toml", "setup.py", "setup.cfg")
		if r.Shell != "" {
			break
		}
		if !r.All {
			f.PythonFiles = coverage.PythonTests(f.Root, r.Tests)
			if len(f.PythonFiles) == 0 {
				break
			}
			f.PythonModules = coverage.PythonModules(f.PythonFiles)
		}
		f.Interpreter = PythonInterpreter(f.Root)
		probe := exec.CommandContext(ctx, f.Interpreter, "-c", "import pytest") //nolint:gosec // probe the project's observed interpreter, never a downloaded executable
		probe.Dir = t.Dir
		if t.Env != nil {
			probe.Env = t.Env
		}
		f.Pytest = probe.Run() == nil
	case "kotlin":
		f.Module = Base(ctx, r.Path, "build.gradle.kts", "build.gradle", "pom.xml")
		f.Root = f.Module
		if exists(filepath.Join(f.Module, "pom.xml")) {
			f.Runner, f.Reactor = "mvn", f.Module
			if !r.All {
				for dir := filepath.Dir(f.Module); dir != filepath.Dir(dir) && exists(filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
					f.Reactor = dir
				}
			}
		} else {
			f.Root = project.FindUp(ctx, r.Path, "settings.gradle.kts", "settings.gradle", "gradlew")
			// Preserve the original fallback from the module path, not its parent.
			if f.Root == "" {
				f.Root = filepath.Dir(f.Module)
			}
			f.Runner = "gradle"
			if exists(filepath.Join(f.Root, "gradlew")) {
				f.Runner = "./gradlew"
			}
		}
		if !r.All {
			f.Classes = coverage.KotlinTests(ctx, r.Tests)
		}
	}
	if err := context.Cause(ctx); err != nil {
		return mutants.ToolFacts{}, mutants.InputCancelled{Cause: err}
	}
	return f, nil
}

// Base is the existing nearest-marker root, falling back to path's directory.
func Base(ctx context.Context, path string, markers ...string) string {
	if root := project.FindUp(ctx, path, markers...); root != "" {
		return root
	}
	return filepath.Dir(path)
}

// PythonInterpreter preserves last-existing .venv then venv preference.
func PythonInterpreter(root string) string {
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); exists(candidate) {
			py = candidate
		}
	}
	return py
}

func packageDeps(dir string) map[string]bool {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // observe the selected project's optional manifest
	_ = json.Unmarshal(data, &pkg)
	deps := map[string]bool{}
	for k := range pkg.Dependencies {
		deps[k] = true
	}
	for k := range pkg.DevDependencies {
		deps[k] = true
	}
	return deps
}
func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// CoverageRunner reuses the current collector selection without executing it.
func (t Toolchain) CoverageRunner(ctx context.Context, source string, related bool) (string, mutants.InputFailure) {
	if err := context.Cause(ctx); err != nil {
		return "", mutants.InputCancelled{Cause: err}
	}
	var runner string
	if spec := lang.Detect(source); spec != nil {
		switch spec.Name {
		case "typescript":
			runner = coverage.TypeScriptRunner(Base(ctx, source, "package.json"), related)
		case "kotlin":
			runner = coverage.KotlinRunner(Base(ctx, source, "build.gradle.kts", "build.gradle", "pom.xml"))
		}
	}
	if err := context.Cause(ctx); err != nil {
		return "", mutants.InputCancelled{Cause: err}
	}
	return runner, nil
}
