package testinputs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/project"
)

// SuiteTestHashes returns every test file of source's language beneath its
// build root, relative to projectRoot: what a whole-suite outcome of source
// depends on besides its support files (ADR-0016). Go's is its nearest
// module (GoModuleTestHashes). TypeScript's is the nearest package.json,
// Python's the nearest pyproject.toml, setup.py or setup.cfg, Kotlin's
// the Gradle build root, which holds settings.gradle or settings.gradle.kts,
// or else the Maven module, the nearest pom.xml, and Dart's the nearest
// pubspec.yaml, as flutter test finds its tests. Tests are those project
// discovery classifies as tests, so node_modules, .venv and the like are
// never walked, and conftest.py counts; a test beneath a nested build root
// of the language belongs to that root. A source with no build root has
// none.
func SuiteTestHashes(ctx context.Context, source, projectRoot string) (map[string]string, error) {
	spec := lang.Detect(source)
	if spec == nil {
		return map[string]string{}, nil
	}
	if spec.Name == "go" {
		return GoModuleTestHashes(source, projectRoot)
	}
	root := suiteRoot(ctx, spec.Name, source)
	if root == "" {
		return map[string]string{}, nil
	}
	files, err := project.Discover(ctx, []string{root})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, test := range files.Tests {
		if s := lang.Detect(test); s == nil || s.Name != spec.Name || suiteRoot(ctx, spec.Name, test) != root {
			continue
		}
		rel, err := filepath.Rel(projectRoot, test)
		if err != nil {
			return nil, err
		}
		out[filepath.ToSlash(rel)] = FileHash(test)
	}
	return out, nil
}

// suiteRoot is the build root of path, a file of language, as
// SuiteTestHashes names it; "" for none.
func suiteRoot(ctx context.Context, language, path string) string {
	switch language {
	case "typescript":
		return project.FindUp(ctx, path, "package.json")
	case "python":
		return project.FindUp(ctx, path, "pyproject.toml", "setup.py", "setup.cfg")
	case "kotlin":
		if root := project.FindUp(ctx, path, "settings.gradle.kts", "settings.gradle"); root != "" {
			return root
		}
		return project.FindUp(ctx, path, "pom.xml")
	case "dart":
		return project.FindUp(ctx, path, "pubspec.yaml")
	}
	return ""
}

// GoModuleTestHashes returns every Go test file in source's nearest module,
// relative to projectRoot. Nested modules are separate suites and are skipped.
func GoModuleTestHashes(source, projectRoot string) (map[string]string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	module := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(module, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(module)
		if parent == module {
			return map[string]string{}, nil
		}
		module = parent
	}
	return moduleTestsUnder(module, module, projectRoot)
}

func moduleTestsUnder(module, dir, projectRoot string) (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if entry.Name() == ".git" {
				continue
			}
			if path != module {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					continue
				}
			}
			nested, err := moduleTestsUnder(module, path, projectRoot)
			if err != nil {
				return nil, err
			}
			for name, hash := range nested {
				out[name] = hash
			}
			continue
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			rel, err := filepath.Rel(projectRoot, path)
			if err != nil {
				return nil, err
			}
			out[filepath.ToSlash(rel)] = FileHash(path)
		}
	}
	return out, nil
}
