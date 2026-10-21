package domain

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/lang/port"
)

// The tests an outcome records, and whose changes make it stale, are those
// its run runs (slice-38). flutter test runs a Dart package's test files
// under its test/ folder, named *_test.dart, and never those under
// integration_test/ or test_driver/, which need a device (q-76), though
// lang counts them as test code and reach finds them importing a library.
// Every other language's outcomes rest on the tests reach finds, as they
// are.

// dartDeviceDirs are the folders of a Dart package whose files flutter test
// never runs.
var dartDeviceDirs = []string{"integration_test", "test_driver"}

// RunTests is which of tests, the test files that reach the source at path
// as reach finds them, its outcomes record and rest on. A Dart source's are
// the runnable test files under its package's test/ folder, as its
// own-test run passes flutter test, with the support files on their way:
// no runnable test elsewhere, no support file under integration_test/ or
// test_driver/ outside test/, and none at all when no runnable test of
// test/ reaches it, since its run then runs nothing. Its package is the
// folder of the nearest pubspec.yaml, or its own folder without one, as
// the toolchain roots its command. Others' are tests.
func RunTests(ctx context.Context, around port.Surroundings, path string, tests []string) []string {
	if spec := lang.Detect(path); spec == nil || spec.Name != "dart" {
		return tests
	}
	pkg := dartPackage(ctx, around, path)
	if pkg == "" {
		return nil
	}
	var out []string
	runs := false
	for _, test := range tests {
		rel, ok := dartRel(ctx, around, pkg, test)
		switch {
		case !ok:
		case strings.HasSuffix(rel, "_test.dart"):
			if strings.HasPrefix(rel, "test/") {
				out, runs = append(out, test), true
			}
		case strings.HasPrefix(rel, "test/") || !dartOnDevice(rel):
			out = append(out, test)
		}
	}
	if !runs {
		return nil
	}
	return out
}

// Suite is the whole-suite tests of one source now, with their hashes, by
// the broad outcome resting on them: Whole, every test file of its build
// root (Fingerprints.Suite), as a --test-command may run any of them, and
// Runs, those --all-tests runs, its language's runner with no path. Those
// are the files of a Dart package's test/ folder, as flutter test runs
// test/ alone (q-76), and for every other language Whole.
type Suite struct {
	Whole, Runs map[string]string
}

// Of is the tests an outcome decided under scope rests on.
func (s Suite) Of(scope string) map[string]string {
	if scope == ScopeAllTests {
		return s.Runs
	}
	return s.Whole
}

// SuiteOf is the Suite of the source at path, whose build root's test
// files are whole, by their paths from root.
func SuiteOf(ctx context.Context, around port.Surroundings, root, path string, whole map[string]string) Suite {
	s := Suite{Whole: whole, Runs: whole}
	if spec := lang.Detect(path); spec == nil || spec.Name != "dart" || whole == nil {
		return s
	}
	pkg := dartPackage(ctx, around, path)
	s.Runs = map[string]string{}
	for test, hash := range whole {
		if rel, ok := dartRel(ctx, around, pkg, filepath.Join(root, filepath.FromSlash(test))); ok && strings.HasPrefix(rel, "test/") {
			s.Runs[test] = hash
		}
	}
	return s
}

// dartPackage is the folder of the Dart file at path's package, absolute:
// that of the nearest pubspec.yaml, or the file's own without one; "" when
// path cannot be made absolute.
func dartPackage(ctx context.Context, around port.Surroundings, path string) string {
	if pkg := lang.Nearest(ctx, around, path, "pubspec.yaml"); pkg != "" {
		return pkg
	}
	dir, ok := around.Abs(ctx, filepath.Dir(path))
	if !ok {
		return ""
	}
	return dir
}

// dartRel is the path of file from pkg, with forward slashes; false when
// file cannot be made absolute or relative to pkg.
func dartRel(ctx context.Context, around port.Surroundings, pkg, file string) (string, bool) {
	abs, ok := around.Abs(ctx, file)
	if !ok || pkg == "" {
		return "", false
	}
	rel, err := filepath.Rel(pkg, abs)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// dartOnDevice says whether rel, a path from a Dart package outside its
// test/ folder, lies in a folder named for one flutter test never runs, of
// that package or of another.
func dartOnDevice(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts[:len(parts)-1] {
		if slices.Contains(dartDeviceDirs, part) {
			return true
		}
	}
	return false
}
