package porttest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/lang"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
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
	case "dart":
		return s.nearest(ctx, path, "pubspec.yaml")
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

// dartPubspec is what a pubspec.yaml of the memory says of the pub
// workspace, as the adapter reads it.
type dartPubspec struct {
	Resolution   string         `yaml:"resolution"`
	Workspace    []string       `yaml:"workspace"`
	Dependencies map[string]any `yaml:"dependencies"`
}

func (s *Inputs) pubspec(ctx context.Context, dir string) dartPubspec {
	var p dartPubspec
	if yaml.Unmarshal([]byte(s.read(ctx, filepath.Join(dir, "pubspec.yaml"))), &p) != nil {
		p = dartPubspec{}
	}
	return p
}

// lists says whether the workspace root at root lists dir, a listed path
// matched as a glob too.
func (p dartPubspec) lists(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, member := range p.Workspace {
		member = path.Clean(filepath.ToSlash(member))
		if ok, _ := path.Match(member, rel); ok || member == rel {
			return true
		}
	}
	return false
}

// dartRoot is the Dart file's package root, or a workspace member's
// workspace root, with that root's pubspec.yaml, as the adapter's
// DartCoverageRoot finds them.
func (s *Inputs) dartRoot(ctx context.Context, source string) (string, dartPubspec) {
	pkg := s.nearest(ctx, source, "pubspec.yaml")
	if pkg == "" {
		return "", dartPubspec{}
	}
	own := s.pubspec(ctx, pkg)
	if own.Resolution == "workspace" {
		for dir := filepath.Dir(pkg); ; dir = filepath.Dir(dir) {
			if s.Exists(ctx, filepath.Join(dir, "pubspec.yaml")) {
				if root := s.pubspec(ctx, dir); root.lists(dir, pkg) {
					return dir, root
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return pkg, own
}

// dartPlatformDir says whether dir is a Flutter app's platform folder, as
// project's FlutterPlatformDir reads the disk.
func (s *Inputs) dartPlatformDir(ctx context.Context, dir string) bool {
	if !slices.Contains([]string{"android", "ios", "macos", "linux", "windows", "web"}, filepath.Base(dir)) {
		return false
	}
	for _, dep := range s.pubspec(ctx, filepath.Dir(dir)).Dependencies {
		if m, ok := dep.(map[string]any); ok && m["sdk"] == "flutter" {
			return true
		}
	}
	return false
}

// dartFlutter is the flutter coverage's FlutterFor resolves for the
// package at dir: FVM's linked SDK beside the nearest .fvmrc, else the
// PATH's, which the memory always has.
func (s *Inputs) dartFlutter(ctx context.Context, dir string) string {
	if root := s.nearest(ctx, filepath.Join(dir, "x"), ".fvmrc"); root != "" {
		if sdk := filepath.Join(root, ".fvm", "flutter_sdk", "bin", "flutter"); s.Exists(ctx, sdk) {
			return sdk
		}
	}
	return "flutter"
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
	switch language {
	case "kotlin":
		root = s.kotlinRoot(ctx, source)
	case "dart":
		if root, _ = s.dartRoot(ctx, source); root != "" && !inside(project, root) {
			root = s.nearest(ctx, source, "pubspec.yaml")
		}
	default:
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

// pnpmInputs records what pnpm reads of the folder of the nearest
// pnpm-workspace.yaml from root up within project, as the adapter does
// (slice-18): its config files as a package root's are named, its .npmrc
// and .pnpmfile.cjs, and the patch files its patchedDependencies name.
func (s *Inputs) pnpmInputs(ctx context.Context, out map[string]string, root, project string) mutants.InputFailure {
	workspace := ""
	for dir := root; inside(project, dir); dir = filepath.Dir(dir) {
		if s.regular(ctx, filepath.Join(dir, "pnpm-workspace.yaml")) {
			workspace = dir
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if workspace == "" {
		return nil
	}
	var files []string
	for _, path := range s.paths(workspace) {
		if name := filepath.Base(path); filepath.Dir(path) == workspace && (tsConfig(name) || name == ".npmrc" || name == ".pnpmfile.cjs") {
			files = append(files, path)
		}
	}
	settings, _ := readPnpmSettings(s.read(ctx, filepath.Join(workspace, "pnpm-workspace.yaml")))
	patches := slices.Collect(maps.Values(settings.PatchedDependencies))
	var manifest struct {
		Pnpm struct {
			PatchedDependencies map[string]string `json:"patchedDependencies"`
		} `json:"pnpm"`
	}
	if json.Unmarshal([]byte(s.read(ctx, filepath.Join(workspace, "package.json"))), &manifest) == nil {
		patches = slices.AppendSeq(patches, maps.Values(manifest.Pnpm.PatchedDependencies))
	}
	for _, patch := range patches {
		if path := filepath.Join(workspace, filepath.FromSlash(patch)); inside(project, path) && s.regular(ctx, path) {
			files = append(files, path)
		}
	}
	for _, path := range files {
		text, failure := s.required(ctx, path)
		if failure != nil {
			return failure
		}
		if f := lang.Detect(path); f != nil {
			text = mutants.StripAnnotation(text, f.Comment)
		}
		name, _ := filepath.Rel(project, path)
		out[filepath.ToSlash(name)] = inputHash(text)
	}
	return nil
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
		case "dart":
			// settings is the root's pubspec.yaml, naming its workspace.
			var workspace dartPubspec
			_ = yaml.Unmarshal([]byte(settings), &workspace)
			if strings.HasPrefix(part, ".") || part == "node_modules" || s.dartPlatformDir(ctx, dir) ||
				slices.Contains([]string{"build", "coverage", "integration_test", "test_driver"}, part) && s.Exists(ctx, filepath.Join(filepath.Dir(dir), "pubspec.yaml")) ||
				s.Exists(ctx, filepath.Join(dir, "pubspec.yaml")) && !workspace.lists(root, dir) {
				return false
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
	switch language {
	case "kotlin":
		for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
			settings += s.read(ctx, filepath.Join(root, name))
		}
	case "dart":
		settings = s.read(ctx, filepath.Join(root, "pubspec.yaml"))
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
		case "dart":
			include = filepath.Ext(name) == ".dart" || slices.Contains([]string{"pubspec.yaml", "pubspec_overrides.yaml", "dart_test.yaml"}, name) || top && name == "pubspec.lock"
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
	if language == "typescript" {
		if failure := s.pnpmInputs(ctx, out, root, project); failure != nil {
			return nil, failure
		}
	}
	if language == "dart" {
		if fvm := s.nearest(ctx, source, ".fvmrc"); fvm != "" && inside(project, fvm) {
			text, failure := s.required(ctx, filepath.Join(fvm, ".fvmrc"))
			if failure != nil {
				return nil, failure
			}
			name, _ := filepath.Rel(project, filepath.Join(fvm, ".fvmrc"))
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
	case "dart":
		dir := s.nearest(ctx, source, "pubspec.yaml")
		if dir == "" {
			dir = filepath.Dir(s.absolute(source))
		}
		out["@flutter-env"] = s.envHash(s.dartFlutter(ctx, dir), "FLUTTER_ROOT", "PUB_CACHE")
	}
	return out, inputStopped(ctx)
}
