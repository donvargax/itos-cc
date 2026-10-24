package covrun

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/project"
)

// FlutterQuiet are the global flags every flutter command itos-cc starts
// carries first: flutter neither reports analytics nor checks for a newer
// Flutter, so a run contacts nothing for the user (decision 26's spirit),
// as both otherwise reach the network whatever the command does.
var FlutterQuiet = []string{"--suppress-analytics", "--no-version-check"}

// dartPlan measures the Dart package at dir, sources its files, with
// tests, the test files that reach them as DartTests gives them, or with
// flutter test's whole suite when tests is nil, test/ alone as flutter test
// runs it with no path, so never integration_test/, which needs a device
// (q-74, q-76). It writes LCOV with SF: paths relative to dir, so each
// member of a pub workspace is a plan of its own, run in its own
// directory. Line coverage only: no --branch-coverage, whose blocks are not
// decisions. Nothing is downloaded or resolved for it (decision 26):
// --no-pub keeps flutter test from running pub get, and a package pub has
// not resolved, a pure Dart package, or no flutter, runs nothing.
func dartPlan(ctx context.Context, dir, out string, sources, tests []string) Plan {
	report := filepath.Join(out, "lcov.info")
	plan := Plan{
		Language: "dart",
		Dir:      dir,
		Reports:  []string{report},
		Existing: []string{report, filepath.Join(dir, "coverage", "lcov.info")},
		// flutter test honours the pragma, so its report never names
		// these files, loaded or not.
		NotCoverable: dartIgnoredFiles(sources),
	}
	flutter := FlutterFor(ctx, dir)
	switch {
	case !flutterPackage(dir):
		plan.Unsupported = "only Flutter apps and packages are measured, and this Dart package does not depend on the flutter SDK; " +
			"measure it with --coverage-command"
	case flutter == "":
		plan.Unsupported = "flutter is not installed; install Flutter, or measure with --coverage-command"
	case !dartResolved(dir):
		plan.Unsupported = "its dependencies are not resolved, as it has no .dart_tool/package_config.json; " +
			"run flutter pub get, or measure with --coverage-command"
	default:
		// --no-dds: the Dart Development Service flutter test starts to
		// collect coverage connects out, and the LCOV is the same without it.
		command := append(append([]string{flutter}, FlutterQuiet...), "test", "--no-pub", "--no-dds", "--coverage", "--coverage-path="+report)
		plan.Commands = [][]string{append(command, tests...)}
	}
	return plan
}

// DartTests is which of tests, the test files that reach a source, a Dart
// own-test run in dir passes flutter test: those under dir's test/ folder
// named as flutter test finds test files, *_test.dart, relative to dir with
// forward slashes, sorted. integration_test/ and test_driver/ are left out,
// as flutter test with no path leaves them: they need a device (q-76).
func DartTests(dir string, tests []string) []string {
	out := []string{}
	for _, test := range tests {
		rel, err := filepath.Rel(dir, test)
		if err != nil || !strings.HasSuffix(rel, "_test.dart") {
			continue
		}
		if rel = filepath.ToSlash(rel); strings.HasPrefix(rel, "test/") && !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// FlutterFor is the flutter that runs the tests of the Dart package at dir:
// the SDK FVM links at .fvm/flutter_sdk beside the nearest .fvmrc at or
// above dir, as a Gradle wrapper is preferred to gradle; else the flutter
// on the PATH, "flutter"; "" when there is neither. Without
// .fvm/flutter_sdk, fvm install never ran there, and the PATH's runs.
func FlutterFor(ctx context.Context, dir string) string {
	if root := project.FindUp(ctx, filepath.Join(dir, "x"), ".fvmrc"); root != "" {
		if sdk := filepath.Join(root, ".fvm", "flutter_sdk", "bin", "flutter"); exists(sdk) {
			return sdk
		}
	}
	if _, err := exec.LookPath("flutter"); err != nil {
		return ""
	}
	return "flutter"
}

// pubspec is what a pubspec.yaml says of the package's dependencies.
type pubspec struct {
	Dependencies map[string]any `yaml:"dependencies"`
}

// flutterPackage says whether the pubspec.yaml at dir depends on the
// flutter SDK: a Flutter app or package. One that does not parse depends on
// nothing.
func flutterPackage(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "pubspec.yaml")) //nolint:gosec // the selected package's own pubspec.yaml
	if err != nil {
		return false
	}
	var p pubspec
	if yaml.Unmarshal(data, &p) != nil {
		return false
	}
	flutter, ok := p.Dependencies["flutter"].(map[string]any)
	return ok && flutter["sdk"] == "flutter"
}

// dartResolved says whether pub resolved the package at dir: its
// .dart_tool/package_config.json, or, for a pub workspace's member, the
// workspace root's, which .dart_tool/pub/workspace_ref.json names relative
// to its own folder.
func dartResolved(dir string) bool {
	if exists(filepath.Join(dir, ".dart_tool", "package_config.json")) {
		return true
	}
	pub := filepath.Join(dir, ".dart_tool", "pub")
	data, err := os.ReadFile(filepath.Join(pub, "workspace_ref.json")) //nolint:gosec // pub's own record in the selected package
	if err != nil {
		return false
	}
	var ref struct {
		WorkspaceRoot string `json:"workspaceRoot"`
	}
	if json.Unmarshal(data, &ref) != nil || ref.WorkspaceRoot == "" {
		return false
	}
	root := filepath.FromSlash(ref.WorkspaceRoot)
	if !filepath.IsAbs(root) {
		root = filepath.Join(pub, root)
	}
	return exists(filepath.Join(root, ".dart_tool", "package_config.json"))
}

// dartIgnoreFile is the pragma that makes a whole Dart library not
// coverable, as package:coverage, which flutter test applies, finds it on
// any line.
var dartIgnoreFile = regexp.MustCompile(`//\s*coverage:ignore-file[\w\s]*$`)

// dartIgnoredFiles is those of sources whose coverage:ignore-file pragma
// makes them not coverable at all; a file that cannot be read is not.
func dartIgnoredFiles(sources []string) []string {
	var out []string
	for _, s := range sources {
		if dartIgnored(s) {
			out = append(out, s)
		}
	}
	return out
}

func dartIgnored(path string) bool {
	f, err := os.Open(path) //nolint:gosec // a selected Dart source
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read-only
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for lines.Scan() {
		if dartIgnoreFile.Match(lines.Bytes()) {
			return true
		}
	}
	return false
}

// notCoverable is r, the report of p's run, naming each of p's
// NotCoverable files with no line: measured, and nothing in it to cover.
func (p Plan) notCoverable(ctx context.Context, r *coverage.Report) *coverage.Report {
	if len(p.NotCoverable) == 0 {
		return r
	}
	entries := make([]coverage.Entry, len(p.NotCoverable))
	for i, file := range p.NotCoverable {
		entries[i] = coverage.Entry{Path: file}
	}
	return coverage.Merge(r, coverage.Build(ctx, Disk{}, p.NotCoverable, p.Dir, entries))
}
