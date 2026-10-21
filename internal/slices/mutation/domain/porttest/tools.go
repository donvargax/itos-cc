package porttest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func (s *Inputs) nearest(ctx context.Context, path string, markers ...string) string {
	return lang.Nearest(ctx, s, path, markers...)
}
func (s *Inputs) base(ctx context.Context, path string, markers ...string) string {
	if root := s.nearest(ctx, path, markers...); root != "" {
		return root
	}
	return filepath.Dir(s.absolute(path))
}

type nodePackage struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
	PackageManager  string            `json:"packageManager"`
}

func (s *Inputs) node(ctx context.Context, dir string) nodePackage {
	var p nodePackage
	_ = json.Unmarshal([]byte(s.read(ctx, filepath.Join(dir, "package.json"))), &p)
	return p
}
func (p nodePackage) measured(name string) bool {
	return p.Dependencies[name] != "" || p.DevDependencies[name] != ""
}
func (p nodePackage) has(name string) bool {
	_, a := p.Dependencies[name]
	_, b := p.DevDependencies[name]
	return a || b
}
func (s *Inputs) nodeBin(ctx context.Context, root, name string) string {
	if s.Runtime.Platform == "windows" {
		name += ".cmd"
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, "node_modules", ".bin", name)
		if s.Exists(ctx, path) {
			return path
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}
func (s *Inputs) manager(ctx context.Context, root string) string {
	for dir := root; ; dir = filepath.Dir(dir) {
		if name, _, _ := strings.Cut(s.node(ctx, dir).PackageManager, "@"); name != "" {
			return name
		}
		for _, pair := range [][2]string{{"pnpm-lock.yaml", "pnpm"}, {"pnpm-workspace.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"}} {
			if s.Exists(ctx, filepath.Join(dir, pair[0])) {
				return pair[1]
			}
		}
		if filepath.Dir(dir) == dir {
			return "npm"
		}
	}
}
func (s *Inputs) interpreter(ctx context.Context, root string, first bool) string {
	py := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if candidate := filepath.Join(root, venv, "bin", "python"); s.Exists(ctx, candidate) {
			py = candidate
			if first {
				break
			}
		}
	}
	return py
}
func pythonFile(name string) bool {
	return strings.HasSuffix(name, ".py") && (strings.HasPrefix(filepath.Base(name), "test_") || strings.HasSuffix(name, "_test.py"))
}
func (s *Inputs) pythonFiles(root string, tests []string) ([]string, []string) {
	files := []string{}
	var modules []string
	for _, path := range tests {
		if pythonFile(path) && inside(root, path) {
			name := relative(root, path)
			if !slices.Contains(files, name) {
				files = append(files, name)
			}
		}
	}
	slices.Sort(files)
	for _, path := range files {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(path, "src/"), ".py"), "/")
		valid := true
		for _, part := range parts {
			if part == "" {
				valid = false
			}
			for i, r := range part {
				if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
					valid = false
				}
			}
		}
		if valid {
			modules = append(modules, strings.Join(parts, "."))
		}
	}
	return files, modules
}
func (s *Inputs) classes(ctx context.Context, tests []string) map[string][]string {
	out := map[string][]string{}
	for _, path := range tests {
		spec := lang.Detect(path)
		if spec == nil || spec.Name != "kotlin" {
			continue
		}
		module := s.nearest(ctx, path, "build.gradle.kts", "build.gradle", "pom.xml")
		if module == "" {
			continue
		}
		text, ok := s.Read(ctx, path)
		if !ok {
			continue
		}
		f, err := lang.Parse(ctx, s, spec, path, text)
		if err != nil {
			continue
		}
		for _, name := range lang.TestClasses(f) {
			if !slices.Contains(out[module], name) {
				out[module] = append(out[module], name)
			}
		}
		f.Close()
	}
	for module := range out {
		slices.Sort(out[module])
	}
	return out
}

func (s *Inputs) Observe(ctx context.Context, r mutants.CommandRequest) (mutants.ToolFacts, mutants.InputFailure) {
	var f mutants.ToolFacts
	if failure := inputStopped(ctx); failure != nil {
		return f, failure
	}
	spec := lang.Detect(r.Path)
	if spec == nil {
		return f, nil
	}
	f.Language = spec.Name
	switch f.Language {
	case "go":
		f.Root = s.base(ctx, r.Path, "go.mod")
	case "typescript":
		f.Root = s.base(ctx, r.Path, "package.json")
		p := s.node(ctx, f.Root)
		if p.has("vitest") {
			f.Vitest = s.nodeBin(ctx, f.Root, "vitest")
		}
		if p.has("jest") {
			f.Jest = s.nodeBin(ctx, f.Root, "jest")
		}
		if f.Vitest == "" && f.Jest == "" {
			f.PackageManager = s.manager(ctx, f.Root)
		}
	case "python":
		f.Root = s.base(ctx, r.Path, "pyproject.toml", "setup.py", "setup.cfg")
		if r.Shell != "" {
			break
		}
		if !r.All {
			f.PythonFiles, f.PythonModules = s.pythonFiles(f.Root, r.Tests)
			if len(f.PythonFiles) == 0 {
				break
			}
		}
		f.Interpreter = s.interpreter(ctx, f.Root, false)
		if hiddenProgram(f.Interpreter, s.Without, s.Runtime.Platform) {
			break
		}
		var ok bool
		f.Pytest, ok = s.Probes[ProbeKey(f.Interpreter, s.Dir, s.Runtime.Env)]
		if !ok {
			return f, inputError(fmt.Errorf("no independent pytest probe for %s from caller %s", f.Interpreter, s.Dir))
		}
	case "kotlin":
		f.Module = s.base(ctx, r.Path, "build.gradle.kts", "build.gradle", "pom.xml")
		f.Root = f.Module
		if s.Exists(ctx, filepath.Join(f.Module, "pom.xml")) {
			f.Runner, f.Reactor = "mvn", f.Module
			if !r.All {
				for dir := filepath.Dir(f.Module); dir != filepath.Dir(dir) && s.Exists(ctx, filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
					f.Reactor = dir
				}
			}
		} else {
			f.Root = s.nearest(ctx, r.Path, "settings.gradle.kts", "settings.gradle", "gradlew")
			if f.Root == "" {
				f.Root = filepath.Dir(f.Module)
			}
			f.Runner = "gradle"
			if s.Exists(ctx, filepath.Join(f.Root, "gradlew")) {
				f.Runner = "./gradlew"
			}
		}
		if !r.All {
			f.Classes = s.classes(ctx, r.Tests)
		}
	}
	return f, inputStopped(ctx)
}

func (s *Inputs) CoverageRunner(ctx context.Context, path string, related bool) (string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return "", failure
	}
	spec := lang.Detect(path)
	if spec == nil {
		return "", nil
	}
	switch spec.Name {
	case "typescript":
		p := s.node(ctx, s.base(ctx, path, "package.json"))
		if p.Scripts["coverage"] != "" && (!related || !p.measured("vitest") && !p.measured("jest")) {
			return "script", nil
		}
		if p.measured("vitest") {
			return "vitest", nil
		}
		if p.measured("jest") {
			return "jest", nil
		}
		return "c8", nil
	case "kotlin":
		module := s.base(ctx, path, "build.gradle.kts", "build.gradle", "pom.xml")
		if s.Exists(ctx, filepath.Join(module, "pom.xml")) {
			return "maven", nil
		}
		for _, name := range []string{"build.gradle.kts", "build.gradle"} {
			if strings.Contains(s.read(ctx, filepath.Join(module, name)), "kover") {
				return "kover", nil
			}
		}
		return "jacoco", nil
	}
	return "", nil
}
