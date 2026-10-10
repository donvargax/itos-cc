package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/lang"
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
	// source says which files anywhere in the root count, by name.
	source func(name string) bool
	// config says which files directly in the root count, by name.
	config func(name string) bool
	// skip leaves out a directory, besides hidden ones, node_modules and
	// nested build roots: top is true directly in the root.
	skip func(path, name string, top bool) bool
}

// rootCoverageInputs fingerprints the files of source's build root inv
// counts, each by addCoverageInput, and returns the root. A build root
// above the project root is not followed: the project root stands for it.
func rootCoverageInputs(source, projectRoot string, inv rootInventory) (map[string]string, string, error) {
	root := lang.FindUp(source, inv.markers...)
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
			if strings.HasPrefix(name, ".") || name == "node_modules" || inv.skip != nil && inv.skip(path, name, top) {
				return filepath.SkipDir
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
func envInput(more string, keys ...string) string {
	env := []string{more}
	for _, key := range keys {
		env = append(env, key+"="+os.Getenv(key))
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
func PythonCoverageInputs(source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	out, root, err := rootCoverageInputs(source, projectRoot, rootInventory{
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
	out["@python-env"] = envInput(interpreter, "PYTHONPATH", "PYTHONSAFEPATH", "VIRTUAL_ENV", "COVERAGE_RCFILE", "PYTEST_ADDOPTS", "PYTEST_PLUGINS", "PYTEST_DISABLE_PLUGIN_AUTOLOAD")
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
// followed: the project root stands for it.
func TypeScriptCoverageInputs(source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	out, _, err := rootCoverageInputs(source, projectRoot, rootInventory{
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
	out["@node-env"] = envInput("", "NODE_OPTIONS", "NODE_ENV", "NODE_PATH", "TS_NODE_PROJECT")
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
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if spec := lang.Detect(path); spec != nil {
		data = []byte(StripAnnotation(string(data), spec.Comment))
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
