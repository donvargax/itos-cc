package porttest

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
)

func (s *Inputs) Tests(ctx context.Context, root string, tests []string) (map[string]string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return nil, failure
	}
	out := map[string]string{}
	for _, path := range tests {
		text, failure := s.required(ctx, path)
		if failure != nil {
			return nil, failure
		}
		out[relative(root, path)] = inputHash(text)
	}
	return out, nil
}
func (s *Inputs) Support(ctx context.Context, root string, globs []string) (map[string]string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return nil, failure
	}
	if len(globs) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, glob := range globs {
		pattern := filepath.Join(root, filepath.FromSlash(glob))
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, inputError(err)
		}
		paths, _ := s.glob(pattern)
		for _, path := range paths {
			if _, regular := s.Files[path]; !regular {
				continue
			}
			name, err := filepath.Rel(root, path)
			if err != nil {
				return nil, inputError(err)
			}
			text, _ := s.Read(ctx, path)
			out[filepath.ToSlash(name)] = inputHash(string(text))
		}
	}
	return out, inputStopped(ctx)
}
func (s *Inputs) suiteRoot(ctx context.Context, language, path string) string {
	switch language {
	case "go":
		return s.nearest(ctx, path, "go.mod")
	case "typescript":
		return s.nearest(ctx, path, "package.json")
	case "python":
		return s.nearest(ctx, path, "pyproject.toml", "setup.py", "setup.cfg")
	case "kotlin":
		if root := s.nearest(ctx, path, "settings.gradle.kts", "settings.gradle"); root != "" {
			return root
		}
		return s.nearest(ctx, path, "pom.xml")
	}
	return ""
}
func (s *Inputs) Suite(ctx context.Context, source, root string) (map[string]string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return nil, failure
	}
	out := map[string]string{}
	spec := lang.Detect(source)
	if spec == nil {
		return out, nil
	}
	build := s.suiteRoot(ctx, spec.Name, source)
	if build == "" {
		return out, nil
	}
	paths := s.paths(build)
	if spec.Name != "go" {
		files := map[string]string{}
		for path, text := range s.Files {
			rel, err := filepath.Rel(build, path)
			if err == nil {
				files[filepath.ToSlash(rel)] = text
			}
		}
		tree := selporttest.TreeOf(build, files, nil)
		_, discovered, failure := tree.Discover(ctx, []string{build})
		if failure != nil {
			return nil, inputError(failure)
		}
		paths = discovered
	}
	for _, path := range paths {
		testSpec := lang.Detect(path)
		if testSpec == nil || testSpec.Name != spec.Name || !testSpec.IsTest(path) {
			continue
		}
		if spec.Name == "go" {
			if strings.Contains(filepath.ToSlash(path), "/.git/") || s.nearest(ctx, path, "go.mod") != build {
				continue
			}
		} else if s.suiteRoot(ctx, spec.Name, path) != build {
			continue
		}
		text, _ := s.Read(ctx, path)
		name, _ := filepath.Rel(root, path)
		out[filepath.ToSlash(name)] = inputHash(string(text))
	}
	return out, inputStopped(ctx)
}
func (s *Inputs) kotlinRoot(ctx context.Context, path string) string {
	module := s.nearest(ctx, path, "build.gradle.kts", "build.gradle", "pom.xml")
	if module == "" {
		return ""
	}
	if s.Exists(ctx, filepath.Join(module, "pom.xml")) {
		top := module
		for dir := filepath.Dir(module); dir != filepath.Dir(dir) && s.Exists(ctx, filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
			top = dir
		}
		return top
	}
	if root := s.nearest(ctx, path, "settings.gradle.kts", "settings.gradle"); root != "" {
		return root
	}
	return module
}
func (s *Inputs) coverageRoot(ctx context.Context, language, source, project string) string {
	var root string
	if language == "go" {
		for dir := filepath.Dir(s.absolute(source)); ; dir = filepath.Dir(dir) {
			if _, regular := s.Files[filepath.Join(dir, "go.mod")]; regular {
				return dir
			}
			if filepath.Dir(dir) == dir {
				return ""
			}
		}
	}
	if language == "kotlin" {
		root = s.kotlinRoot(ctx, source)
	} else {
		root = s.suiteRoot(ctx, language, source)
	}
	if root == "" || !inside(project, root) {
		root = filepath.Dir(s.absolute(source))
		if !inside(project, root) {
			root = project
		}
	}
	return root
}
func (s *Inputs) envHash(more string, keys ...string) string {
	values := []string{more}
	for _, key := range keys {
		values = append(values, key+"="+s.Runtime.Env[key])
	}
	return inputHash(strings.Join(values, "\x00"))
}

// goSkips is whether go's directory rules leave out a directory or file of
// this name.
func goSkips(name string) bool {
	return name == "testdata" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// memoryGoVersion is GOVERSION as the memory's one Go toolchain resolves it.
const memoryGoVersion = "go-memory"

// goEnv is the Go environment as go env resolves it for the memory's one
// toolchain, in the adapter's order: a variable the run sets, else its
// default, GOOS and GOARCH the platform's. GOTOOLCHAIN only says how the
// version was chosen, so it is no part of it.
func (s *Inputs) goEnv() []string {
	defaults := map[string]string{"GOHOSTOS": s.Runtime.Platform, "GOHOSTARCH": s.Runtime.Architecture, "GOOS": s.Runtime.Platform, "GOARCH": s.Runtime.Architecture}
	env := []string{"GOVERSION=" + memoryGoVersion}
	for _, key := range []string{"GOHOSTOS", "GOHOSTARCH", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOEXPERIMENT", "GOAMD64", "GOARM", "GO386"} {
		value := s.Runtime.Env[key]
		if value == "" || key == "GOHOSTOS" || key == "GOHOSTARCH" {
			value = defaults[key]
		}
		env = append(env, key+"="+value)
	}
	return env
}

func tsConfig(name string) bool {
	return slices.Contains([]string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "yarn.lock", "bun.lock", "bun.lockb", ".babelrc", ".swcrc", "jest.config.json", "vitest.workspace.json"}, name) || strings.HasPrefix(name, "tsconfig") && strings.HasSuffix(name, ".json")
}
func kotlinInput(name string) bool {
	return slices.Contains([]string{".kt", ".kts", ".java", ".gradle"}, filepath.Ext(name)) || slices.Contains([]string{"pom.xml", "gradle.properties", "gradle-wrapper.properties", "maven.config", "jvm.config", "extensions.xml"}, name) || strings.HasSuffix(name, ".versions.toml")
}
func (s *Inputs) admittedPath(ctx context.Context, language, root, path, settings string) bool {
	rel, _ := filepath.Rel(root, path)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	dir := root
	for i, part := range parts[:len(parts)-1] {
		dir = filepath.Join(dir, part)
		switch language {
		case "go":
			if goSkips(part) || s.Exists(ctx, filepath.Join(dir, "go.mod")) {
				return false
			}
		case "python":
			if strings.HasPrefix(part, ".") || part == "node_modules" || part == "__pycache__" || s.Exists(ctx, filepath.Join(dir, "pyvenv.cfg")) {
				return false
			}
			for _, marker := range []string{"pyproject.toml", "setup.py", "setup.cfg"} {
				if s.Exists(ctx, filepath.Join(dir, marker)) {
					return false
				}
			}
		case "typescript":
			if strings.HasPrefix(part, ".") || part == "node_modules" || i == 0 && slices.Contains([]string{"dist", "build", "out", "coverage"}, part) || s.Exists(ctx, filepath.Join(dir, "package.json")) {
				return false
			}
		case "kotlin":
			if strings.HasPrefix(part, ".") && part != ".mvn" || part == "node_modules" {
				return false
			}
			if part == "build" || part == "target" {
				for _, marker := range []string{"build.gradle.kts", "build.gradle", "pom.xml", "settings.gradle.kts", "settings.gradle"} {
					if s.Exists(ctx, filepath.Join(filepath.Dir(dir), marker)) {
						return false
					}
				}
			}
			if part != "buildSrc" && (s.Exists(ctx, filepath.Join(dir, "settings.gradle.kts")) || s.Exists(ctx, filepath.Join(dir, "settings.gradle"))) {
				nested, _ := filepath.Rel(root, dir)
				if !strings.Contains(settings, filepath.ToSlash(nested)) {
					return false
				}
			}
		}
	}
	return true
}
func (s *Inputs) Coverage(ctx context.Context, source, project, producer string, support map[string]string) (map[string]string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return nil, failure
	}
	spec := lang.Detect(source)
	if spec == nil {
		return nil, nil
	}
	language := spec.Name
	root := s.coverageRoot(ctx, language, source, project)
	if language == "go" && root == "" {
		return nil, inputError(fmt.Errorf("%s: no nearest go.mod", source))
	}
	var settings string
	if language == "kotlin" {
		for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
			settings += s.read(ctx, filepath.Join(root, name))
		}
	}
	out := map[string]string{}
	for _, path := range s.paths(root) {
		if !s.admittedPath(ctx, language, root, path, settings) {
			continue
		}
		name := filepath.Base(path)
		top := filepath.Dir(path) == root
		include := false
		switch language {
		case "go":
			// go.mod and go.sum are the module's own; a Go file counts when
			// a build can include it, and, in a repository, when git tracks it.
			include = top && (name == "go.mod" || name == "go.sum") ||
				strings.HasSuffix(name, ".go") && !goSkips(name) && (s.Tracked == nil || s.Tracked[path])
		case "python":
			include = strings.HasSuffix(name, ".py") || top && slices.Contains([]string{"pyproject.toml", "setup.cfg", "tox.ini", "pytest.ini", ".coveragerc"}, name)
		case "typescript":
			include = slices.Contains([]string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}, filepath.Ext(name)) || top && tsConfig(name)
		case "kotlin":
			include = kotlinInput(name)
		}
		if include {
			text, failure := s.required(ctx, path)
			if failure != nil {
				return nil, failure
			}
			if f := lang.Detect(path); f != nil {
				text = mutants.StripAnnotation(text, f.Comment)
			}
			name, _ := filepath.Rel(project, path)
			out[filepath.ToSlash(name)] = inputHash(text)
		}
	}
	if text, ok := s.Read(ctx, filepath.Join(project, "itos-cc.yaml")); ok {
		out["itos-cc.yaml"] = inputHash(string(text))
	} else {
		out["itos-cc.yaml"] = "absent"
	}
	for path, hash := range support {
		out["support:"+path] = hash
	}
	out["@producer"] = inputHash(producer)
	switch language {
	case "go":
		out["@go-env"] = inputHash(strings.Join(s.goEnv(), "\x00"))
	case "python":
		out["@python-env"] = s.envHash(s.interpreter(ctx, root, true), "PYTHONPATH", "PYTHONSAFEPATH", "VIRTUAL_ENV", "COVERAGE_RCFILE", "PYTEST_ADDOPTS", "PYTEST_PLUGINS", "PYTEST_DISABLE_PLUGIN_AUTOLOAD")
	case "typescript":
		out["@node-env"] = s.envHash("", "NODE_OPTIONS", "NODE_ENV", "NODE_PATH", "TS_NODE_PROJECT")
	case "kotlin":
		runner := "mvn"
		module := s.nearest(ctx, source, "build.gradle.kts", "build.gradle", "pom.xml")
		if module != "" && !s.Exists(ctx, filepath.Join(module, "pom.xml")) {
			runner = "gradle"
			if s.nearest(ctx, filepath.Join(module, "x"), "gradlew") != "" {
				runner = "gradlew"
			}
		}
		out["@jvm-env"] = s.envHash(runner, "JAVA_HOME", "GRADLE_OPTS", "MAVEN_OPTS", "JAVA_TOOL_OPTIONS")
	}
	return out, inputStopped(ctx)
}
