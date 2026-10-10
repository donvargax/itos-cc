package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
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
	root := lang.FindUp(source, pythonRootMarkers...)
	if root == "" || !beneathRoot(projectRoot, root) {
		abs, err := filepath.Abs(source)
		if err != nil {
			return nil, err
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
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || name == "__pycache__" || name == "node_modules" ||
				coverageFileExists(filepath.Join(path, "pyvenv.cfg")) {
				return filepath.SkipDir
			}
			for _, marker := range pythonRootMarkers {
				if coverageFileExists(filepath.Join(path, marker)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(name, ".py") || filepath.Dir(path) == root && pythonConfigFiles[name] {
			return addCoverageInput(out, projectRoot, path)
		}
		return nil
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
	var env []string
	for _, key := range []string{"PYTHONPATH", "PYTHONSAFEPATH", "VIRTUAL_ENV", "COVERAGE_RCFILE", "PYTEST_ADDOPTS", "PYTEST_PLUGINS", "PYTEST_DISABLE_PLUGIN_AUTOLOAD"} {
		env = append(env, key+"="+os.Getenv(key))
	}
	out["@python-env"] = hashText(interpreter + "\x00" + strings.Join(env, "\x00"))
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
