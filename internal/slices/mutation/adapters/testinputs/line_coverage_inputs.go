package testinputs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// pythonRootMarkers mark a Python build root, as coverage plans find it.
var pythonRootMarkers = []string{"pyproject.toml", "setup.py", "setup.cfg"}

// pythonConfigFiles are the files beside the Python sources that configure
// how pytest, unittest and coverage.py run and what they measure.
var pythonConfigFiles = map[string]bool{
	"pyproject.toml": true, "setup.cfg": true, "tox.ini": true, "pytest.ini": true, ".coveragerc": true,
}

// rootInventory is which files of a build root a line-precision
// language's strict coverage evidence rests on.
type rootInventory struct {
	// markers mark a build root, as coverage plans find it.
	markers []string
	// root, when set, finds source's build root, in place of the nearest
	// directory holding a marker.
	root func(source string) string
	// source says which files anywhere in the root count, by name.
	source func(name string) bool
	// config says which files directly in the root count, by name.
	config func(name string) bool
	// skip leaves out a directory, besides hidden ones, node_modules and
	// nested build roots: top is true directly in the root.
	skip func(path, name string, top bool) bool
	// nested, when set, says which directories below the root are other
	// build roots, in place of those holding a marker.
	nested func(path string) bool
	// hidden are the hidden directories that count all the same.
	hidden []string
}

// rootCoverageInputs fingerprints the files of source's build root inv
// counts, each by addCoverageInput, and returns the root. A build root
// above the project root is not followed: the project root stands for it.
func rootCoverageInputs(ctx context.Context, source, projectRoot string, inv rootInventory) (map[string]string, string, error) {
	var root string
	if inv.root != nil {
		root = inv.root(source)
	} else {
		root = project.FindUp(ctx, source, inv.markers...)
	}
	if root == "" || !beneathRoot(projectRoot, root) {
		abs, err := filepath.Abs(source)
		if err != nil {
			return nil, "", err
		}
		if root = filepath.Dir(abs); !beneathRoot(projectRoot, root) {
			root = projectRoot
		}
	}
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		top := filepath.Dir(path) == root
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") && !slices.Contains(inv.hidden, name) || name == "node_modules" ||
				inv.skip != nil && inv.skip(path, name, top) {
				return filepath.SkipDir
			}
			if inv.nested != nil {
				if inv.nested(path) {
					return filepath.SkipDir
				}
				return nil
			}
			for _, marker := range inv.markers {
				if coverageFileExists(filepath.Join(path, marker)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if inv.source(name) || top && inv.config(name) {
			return addCoverageInput(out, projectRoot, path)
		}
		return nil
	})
	return out, root, err
}

// envInput hashes the values of keys in the environment, beside more.
func envInput(runtime mutants.RuntimeInputs, more string, keys ...string) string {
	env := []string{more}
	for _, key := range keys {
		env = append(env, key+"="+runtime.Env[key])
	}
	return hashText(strings.Join(env, "\x00"))
}

// PythonCoverageInputs fingerprints the conservative input boundary of
// independently measured Python line coverage of source: every Python file
// of its build root, the tests that reach it and the code they import
// included, each without the summary comment itos-cc writes, the root's
// pytest and coverage.py configuration, the project's itos-cc.yaml, the
// configured support files, the producer and the interpreter a coverage
// plan runs with its environment. A directory that is a virtualenv
// (pyvenv.cfg), hidden, __pycache__, node_modules or another build root is
// left out. A build root above the project root is not followed: the
// project root stands for it.
func PythonCoverageInputs(ctx context.Context, runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	out, root, err := rootCoverageInputs(ctx, source, projectRoot, rootInventory{
		markers: pythonRootMarkers,
		source:  func(name string) bool { return strings.HasSuffix(name, ".py") },
		config:  func(name string) bool { return pythonConfigFiles[name] },
		skip: func(path, name string, _ bool) bool {
			return name == "__pycache__" || coverageFileExists(filepath.Join(path, "pyvenv.cfg"))
		},
	})
	if err != nil {
		return nil, err
	}
	interpreter := "python3"
	for _, venv := range []string{".venv", "venv"} {
		if py := filepath.Join(root, venv, "bin", "python"); coverageFileExists(py) {
			interpreter = py
			break
		}
	}
	out["@python-env"] = envInput(runtime, interpreter, "PYTHONPATH", "PYTHONSAFEPATH", "VIRTUAL_ENV", "COVERAGE_RCFILE", "PYTEST_ADDOPTS", "PYTEST_PLUGINS", "PYTEST_DISABLE_PLUGIN_AUTOLOAD")
	if err := addProjectCoverageInputs(out, projectRoot, producer, support); err != nil {
		return nil, err
	}
	return out, nil
}

// typescriptSources are the extensions of the files a TypeScript package's
// tests and sources are written in.
var typescriptSources = []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

// typescriptConfig says whether a file directly in a package root
// configures how its tests, its build or its coverage run: package.json,
// its lockfile, tsconfig*.json, and the JSON configuration of Babel, SWC,
// Vitest and Jest, whose script configuration counts as a source.
func typescriptConfig(name string) bool {
	switch name {
	case "package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "pnpm-workspace.yaml",
		"yarn.lock", "bun.lock", "bun.lockb", ".babelrc", ".swcrc", "jest.config.json", "vitest.workspace.json":
		return true
	}
	return strings.HasPrefix(name, "tsconfig") && strings.HasSuffix(name, ".json")
}

// TypeScriptCoverageInputs fingerprints the conservative input boundary of
// independently measured TypeScript line coverage of source: every
// TypeScript and JavaScript file of its package root (the nearest
// package.json), sources and tests, Vitest, Jest and Vite configuration
// included, each without the summary comment itos-cc writes, the root's
// package.json, lockfile, tsconfig*.json and Babel, SWC, Vitest and Jest
// JSON configuration, the project's itos-cc.yaml, the configured support
// files, the producer and the Node environment. node_modules, hidden
// directories, nested packages and the root's dist, build, out and coverage
// outputs are left out. A package root above the project root is not
// followed: the project root stands for it. Under a pnpm-workspace.yaml, the
// nearest from the package root up within the project root, what pnpm reads
// of its folder counts too (slice-18): its config files as a package root's
// are named, its .npmrc and .pnpmfile.cjs, and the patch files its
// patchedDependencies name, there or in its package.json's pnpm field. The
// workspace's other members are no input.
func TypeScriptCoverageInputs(ctx context.Context, runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	out, root, err := rootCoverageInputs(ctx, source, projectRoot, rootInventory{
		markers: []string{"package.json"},
		source:  func(name string) bool { return slices.Contains(typescriptSources, filepath.Ext(name)) },
		config:  typescriptConfig,
		skip: func(_, name string, top bool) bool {
			return top && (name == "dist" || name == "build" || name == "out" || name == "coverage")
		},
	})
	if err != nil {
		return nil, err
	}
	if err := addPnpmCoverageInputs(out, root, projectRoot); err != nil {
		return nil, err
	}
	out["@node-env"] = envInput(runtime, "", "NODE_OPTIONS", "NODE_ENV", "NODE_PATH", "TS_NODE_PROJECT")
	if err := addProjectCoverageInputs(out, projectRoot, producer, support); err != nil {
		return nil, err
	}
	return out, nil
}

// pnpmConfig says whether a file directly in the folder of a
// pnpm-workspace.yaml configures how pnpm installs a package beneath it.
func pnpmConfig(name string) bool {
	return typescriptConfig(name) || name == ".npmrc" || name == ".pnpmfile.cjs"
}

// addPnpmCoverageInputs records what pnpm reads of the folder of the
// nearest pnpm-workspace.yaml from root up within projectRoot, if any: its
// pnpmConfig files and the patch files patchedDependencies names beneath
// projectRoot.
func addPnpmCoverageInputs(inputs map[string]string, root, projectRoot string) error {
	workspace := ""
	for dir := root; beneathRoot(projectRoot, dir); dir = filepath.Dir(dir) {
		if coverageFileExists(filepath.Join(dir, "pnpm-workspace.yaml")) {
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
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if path := filepath.Join(workspace, entry.Name()); pnpmConfig(entry.Name()) && coverageFileExists(path) {
			if err := addCoverageInput(inputs, projectRoot, path); err != nil {
				return err
			}
		}
	}
	for _, patch := range pnpmPatches(workspace) {
		path := filepath.Join(workspace, filepath.FromSlash(patch))
		if beneathRoot(projectRoot, path) && coverageFileExists(path) {
			if err := addCoverageInput(inputs, projectRoot, path); err != nil {
				return err
			}
		}
	}
	return nil
}

// pnpmPatches are the patch files the pnpm settings in dir name, as its
// folder's paths: patchedDependencies in its pnpm-workspace.yaml or in its
// package.json's pnpm field.
func pnpmPatches(dir string) []string {
	var patches []string
	if data, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil { //nolint:gosec // the pnpm settings above the package a strict file belongs to, within the project root
		settings, _ := readPnpmSettings(string(data))
		patches = slices.AppendSeq(patches, maps.Values(settings.PatchedDependencies))
	}
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil { //nolint:gosec // the package.json beside those pnpm settings
		var manifest struct {
			Pnpm struct {
				PatchedDependencies map[string]string `json:"patchedDependencies"`
			} `json:"pnpm"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			patches = slices.AppendSeq(patches, maps.Values(manifest.Pnpm.PatchedDependencies))
		}
	}
	slices.Sort(patches)
	return slices.Compact(patches)
}

// kotlinModuleMarkers mark a Kotlin build module, as coverage plans find
// it, and kotlinSettings a Gradle build root.
var (
	kotlinModuleMarkers = []string{"build.gradle.kts", "build.gradle", "pom.xml"}
	kotlinSettings      = []string{"settings.gradle.kts", "settings.gradle"}
)

// kotlinInput says whether a file anywhere in a Kotlin build root is an
// input of its coverage: a Kotlin or Java source or test, a Gradle script
// (build and settings scripts, in Kotlin or Groovy, and scripts they
// apply), gradle.properties, a version catalog, the Gradle wrapper's
// properties, a Maven pom.xml, or Maven's own configuration in .mvn.
func kotlinInput(name string) bool {
	switch filepath.Ext(name) {
	case ".kt", ".kts", ".java", ".gradle":
		return true
	}
	switch name {
	case "pom.xml", "gradle.properties", "gradle-wrapper.properties", "maven.config", "jvm.config", "extensions.xml":
		return true
	}
	return strings.HasSuffix(name, ".versions.toml")
}

// KotlinBuildRoot is the build root of the Kotlin source at source, or ""
// outside any build: for a Maven module, the top of its reactor, the
// highest of the directories holding a pom.xml from the module up without
// a gap; for Gradle, the nearest directory with a settings script, else the
// module's own directory.
func KotlinBuildRoot(ctx context.Context, source string) string {
	module := project.FindUp(ctx, source, kotlinModuleMarkers...)
	if module == "" {
		return ""
	}
	if coverageFileExists(filepath.Join(module, "pom.xml")) {
		top := module
		for dir := filepath.Dir(module); dir != filepath.Dir(dir) && coverageFileExists(filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
			top = dir
		}
		return top
	}
	if root := project.FindUp(ctx, source, kotlinSettings...); root != "" {
		return root
	}
	return module
}

// KotlinCoverageInputs fingerprints the conservative input boundary of
// independently measured Kotlin line coverage of source: every input
// (kotlinInput) of its whole build root (KotlinBuildRoot), every module of
// a multi-module build included, each source without the summary comment
// itos-cc writes, the project's itos-cc.yaml, the configured support files,
// the producer and the JVM environment with whether Gradle runs through its
// wrapper. A module's coverage plan runs in the build its settings or its
// reactor make, which compiles the modules it depends on, and its tests
// reach code anywhere in it, so a change anywhere in the build stales it.
// Each module's build and target output directories, hidden directories
// (but .mvn), node_modules and nested Gradle builds, a directory below the
// root with settings of its own other than buildSrc or a build the root's
// settings name (includeBuild), are left out. A build root above the
// project root is not followed: the project root stands for it.
func KotlinCoverageInputs(ctx context.Context, runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	build := KotlinBuildRoot(ctx, source)
	if build != "" && !beneathRoot(projectRoot, build) {
		build = projectRoot
	}
	var settings string
	for _, name := range kotlinSettings {
		if build == "" {
			break
		}
		if data, err := os.ReadFile(filepath.Join(build, name)); err == nil { //nolint:gosec // fingerprint actual files within the admitted language build inventory
			settings += string(data)
		}
	}
	out, _, err := rootCoverageInputs(ctx, source, projectRoot, rootInventory{
		root:   func(string) string { return build },
		source: kotlinInput,
		config: func(string) bool { return false },
		skip: func(path, name string, _ bool) bool {
			if name != "build" && name != "target" {
				return false
			}
			for _, marker := range append(slices.Clone(kotlinModuleMarkers), kotlinSettings...) {
				if coverageFileExists(filepath.Join(filepath.Dir(path), marker)) {
					return true
				}
			}
			return false
		},
		nested: func(path string) bool {
			if filepath.Base(path) == "buildSrc" {
				return false
			}
			for _, marker := range kotlinSettings {
				if coverageFileExists(filepath.Join(path, marker)) {
					rel, err := filepath.Rel(build, path)
					return err != nil || !strings.Contains(settings, filepath.ToSlash(rel))
				}
			}
			return false
		},
		hidden: []string{".mvn"},
	})
	if err != nil {
		return nil, err
	}
	runner := "mvn"
	if module := project.FindUp(ctx, source, kotlinModuleMarkers...); module != "" && coverage.KotlinRunner(module) != "maven" {
		runner = "gradle"
		if coverage.GradleWrapper(ctx, module) {
			runner = "gradlew"
		}
	}
	out["@jvm-env"] = envInput(runtime, runner, "JAVA_HOME", "GRADLE_OPTS", "MAVEN_OPTS", "JAVA_TOOL_OPTIONS")
	if err := addProjectCoverageInputs(out, projectRoot, producer, support); err != nil {
		return nil, err
	}
	return out, nil
}

// dartConfigFiles are the files anywhere in a Dart package's root, beside
// its Dart, that say how it resolves and how its tests run.
var dartConfigFiles = map[string]bool{"pubspec.yaml": true, "pubspec_overrides.yaml": true, "dart_test.yaml": true}

// dartLeftOut are the folders beside a pubspec.yaml whose files are no
// input of its coverage: flutter's build output, the coverage it writes,
// and the tests flutter test never runs, which need a device (q-76).
var dartLeftOut = map[string]bool{"build": true, "coverage": true, "integration_test": true, "test_driver": true}

// pubspecOf is what a pubspec.yaml says of the pub workspace: the members
// a workspace root lists, and a member's resolution: workspace. One that
// does not parse says neither.
type pubspecOf struct {
	Resolution string   `yaml:"resolution"`
	Workspace  []string `yaml:"workspace"`
}

func readPubspec(dir string) pubspecOf {
	var p pubspecOf
	data, err := os.ReadFile(filepath.Join(dir, "pubspec.yaml")) //nolint:gosec // a pubspec.yaml of the package a strict file belongs to, or above it
	if err == nil && yaml.Unmarshal(data, &p) != nil {
		p = pubspecOf{}
	}
	return p
}

// lists says whether the workspace root at root lists dir as a member. A
// listed path is matched as a glob too, so a pattern names each folder it
// matches.
func (p pubspecOf) lists(root, dir string) bool {
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

// DartCoverageRoot is the root the strict coverage evidence of the Dart
// file at source rests on, with the pub workspace that root heads: its
// package root, the nearest pubspec.yaml, or, for a workspace member, one
// whose pubspec.yaml says resolution: workspace, the workspace root, the
// nearest pubspec.yaml above it that lists it under workspace:. "" outside
// any package.
func DartCoverageRoot(ctx context.Context, source string) (string, pubspecOf) {
	pkg := project.FindUp(ctx, source, "pubspec.yaml")
	if pkg == "" {
		return "", pubspecOf{}
	}
	own := readPubspec(pkg)
	if own.Resolution == "workspace" {
		for dir := filepath.Dir(pkg); ; dir = filepath.Dir(dir) {
			if coverageFileExists(filepath.Join(dir, "pubspec.yaml")) {
				if root := readPubspec(dir); root.lists(dir, pkg) {
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

// dartNested says whether dir, a folder beneath root, holds another
// package: a pubspec.yaml that is not a member root's workspace lists.
func dartNested(root string, workspace pubspecOf, dir string) bool {
	return coverageFileExists(filepath.Join(dir, "pubspec.yaml")) && !workspace.lists(root, dir)
}

// DartCoverageInputs fingerprints the conservative input boundary of
// independently measured Dart line coverage of source (q-B, q-E, q-F):
// every Dart file of its root (DartCoverageRoot), the whole pub workspace
// for a member, generated Dart included, since tests compile against it,
// each without the summary comment itos-cc writes; each pubspec.yaml,
// pubspec_overrides.yaml and dart_test.yaml there; the root's pubspec.lock;
// the nearest .fvmrc beneath the project root; the project's itos-cc.yaml,
// the configured support files, the producer, and the flutter that runs,
// as coverage.FlutterFor resolves it, with FLUTTER_ROOT and PUB_CACHE. No
// command runs for them. Hidden folders, .dart_tool among them, build/,
// coverage/, integration_test/ and test_driver/ beside a pubspec.yaml, a
// Flutter app's platform folders and a nested package the workspace does
// not list are left out. A workspace root above the project root is not
// followed: the package root stands for it, and the project root for a
// package root above it too.
func DartCoverageInputs(ctx context.Context, runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	pkg := project.FindUp(ctx, source, "pubspec.yaml")
	root, workspace := DartCoverageRoot(ctx, source)
	if root != "" && !beneathRoot(projectRoot, root) {
		root, workspace = pkg, readPubspec(pkg)
		if !beneathRoot(projectRoot, root) {
			root = ""
		}
	}
	out, _, err := rootCoverageInputs(ctx, source, projectRoot, rootInventory{
		root:   func(string) string { return root },
		source: func(name string) bool { return filepath.Ext(name) == ".dart" || dartConfigFiles[name] },
		config: func(name string) bool { return name == "pubspec.lock" },
		skip: func(path, name string, _ bool) bool {
			if dartLeftOut[name] && coverageFileExists(filepath.Join(filepath.Dir(path), "pubspec.yaml")) {
				return true
			}
			return project.FlutterPlatformDir(path)
		},
		nested: func(path string) bool { return dartNested(root, workspace, path) },
	})
	if err != nil {
		return nil, err
	}
	if fvm := project.FindUp(ctx, source, ".fvmrc"); fvm != "" && beneathRoot(projectRoot, fvm) {
		if err := addCoverageInput(out, projectRoot, filepath.Join(fvm, ".fvmrc")); err != nil {
			return nil, err
		}
	}
	dir := pkg
	if dir == "" {
		dir = filepath.Dir(source)
	}
	out["@flutter-env"] = envInput(runtime, coverage.FlutterFor(ctx, dir), "FLUTTER_ROOT", "PUB_CACHE")
	if err := addProjectCoverageInputs(out, projectRoot, producer, support); err != nil {
		return nil, err
	}
	return out, nil
}

// addCoverageInput records the SHA-256 of the file at path in inputs, by
// its slash-separated path from projectRoot, a source file's without the
// summary comment itos-cc writes, so a run's own summary does not stale
// its evidence.
func addCoverageInput(inputs map[string]string, projectRoot, path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // fingerprint actual files within the admitted language build inventory
	if err != nil {
		return err
	}
	if spec := lang.Detect(path); spec != nil {
		data = []byte(mutants.StripAnnotation(string(data), spec.Comment))
	}
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	inputs[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
	return nil
}

// addProjectCoverageInputs records the inputs of every language's strict
// coverage evidence beyond its build root: the project's itos-cc.yaml, or
// that it has none, the configured support files and the producer.
func addProjectCoverageInputs(inputs map[string]string, projectRoot, producer string, support map[string]string) error {
	if config := filepath.Join(projectRoot, "itos-cc.yaml"); coverageFileExists(config) {
		if err := addCoverageInput(inputs, projectRoot, config); err != nil {
			return err
		}
	} else {
		inputs["itos-cc.yaml"] = "absent"
	}
	for path, hash := range support {
		inputs["support:"+path] = hash
	}
	inputs["@producer"] = hashText(producer)
	return nil
}

// beneathRoot says whether path is root or beneath it.
func beneathRoot(root, path string) bool {
	root, err1 := filepath.Abs(root)
	path, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func coverageFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
