package features

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

// binary is the driver that runs itos-cc as another process, the binary at
// bin TestFeatures built from this tree, the module at root, in folders of
// the system's temporary directory, with the real git and the project's
// own tools: what proves main's wiring, the real adapters, exit codes and
// signals (decision 44).
type binary struct {
	bin, root string
}

var _ Driver = binary{}

// Folder makes a fresh folder in the system's temporary directory.
func (binary) Folder() (string, error) {
	return os.MkdirTemp("", "itos-cc-features-")
}

// Write writes data to path, making the folders above it.
func (binary) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // a scenario's folders have the 0755 a checkout's folders have; the product under test sees fixture modes, so narrowing them would change what scenarios check
		return err
	}
	return os.WriteFile(path, data, 0o600) //nolint:gosec // a scenario file, at a path its steps name in the scenario's own temporary folder
}

func (b binary) WriteExecutable(path string, data []byte) error {
	if err := b.Write(path, data); err != nil {
		return err
	}
	return os.Chmod(path, 0o700) //nolint:gosec // an executable fixture, such as a test script, must be executable by its owner
}

// Read is the content of the file at path.
func (binary) Read(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // a file of the scenario's own temporary folder, as its steps name it
}

// Mkdir makes the folder at path.
func (binary) Mkdir(path string) error {
	return os.Mkdir(path, 0o755) //nolint:gosec // a scenario's folders have the 0755 a checkout's folders have; the product under test sees fixture modes, so narrowing them would change what scenarios check
}

// Exists is whether anything is at path.
func (binary) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Remove removes path and everything below it.
func (binary) Remove(path string) error {
	return os.RemoveAll(path)
}

// Git runs the real git with args in dir, in the scenarios' environment.
func (binary) Git(dir string, args ...string) error {
	bin, err := realGit()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // the real git the scenarios resolved, with the arguments a step names
	cmd.Dir = dir
	cmd.Env = environment(nil)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// The real TypeScript and Python tools a scenario declares (Coverage) are
// the ones CI installs, pinned, on ubuntu-latest alone (T-13): Node's from
// testdata/tools/node with npm ci, Python's from
// testdata/tools/python/requirements.txt. Elsewhere a scenario that needs
// one skips, naming it, as cmd/itos-cc's languageTools does.
const (
	// languageToolsEnv, set by the job that installed the pinned tools,
	// turns a missing tool into a failure, so a broken install is never a
	// skip.
	languageToolsEnv = "ITOS_CC_LANGUAGE_TOOLS"
	// nodeModulesEnv names a node_modules that holds vitest and
	// @vitest/coverage-v8.
	nodeModulesEnv = "ITOS_CC_NODE_MODULES"
)

// Coverage checks the tools c's language measures with are installed, and
// for TypeScript links into dir the node_modules holding vitest and
// @vitest/coverage-v8: the one ITOS_CC_NODE_MODULES names, else
// testdata/tools/node's, else the viewer's. Go needs only go; TypeScript
// node; Python python3 with pytest and coverage importable. A tool missing
// skips the scenario, or fails it under ITOS_CC_LANGUAGE_TOOLS. A CI job
// that did not install the pinned tools skips a scenario needing node or
// python: its runner's preinstalled ones are whatever version its image has.
func (b binary) Coverage(dir string, c Coverage) error {
	if err := c.Validate(); err != nil {
		return err
	}
	required := os.Getenv(languageToolsEnv) != ""
	if err := languageJob(c.Language); err != nil {
		return err
	}
	switch c.Language {
	case "go":
		if _, err := exec.LookPath("go"); err != nil {
			return missing("go")
		}
	case "typescript":
		if _, err := exec.LookPath("node"); err != nil {
			return missing("node")
		}
		modules := b.nodeModules()
		if modules == "" {
			return missing("vitest with @vitest/coverage-v8 (npm ci in testdata/tools/node, or " + nodeModulesEnv + ")")
		}
		if strings.TrimSpace(c.IntegrationReport) != "" {
			if _, err := os.Stat(filepath.Join(modules, "c8", "package.json")); os.IsNotExist(err) {
				return missing("c8 for subprocess coverage")
			} else if err != nil {
				return err
			}
		}
		if c.WithoutC8 {
			if err := coverageWithoutC8(modules, filepath.Join(dir, "node_modules"), runtime.GOOS, required); err != nil {
				return err
			}
		} else if err := linkModules(modules, filepath.Join(dir, "node_modules"), func(err error) error {
			if required {
				return err
			}
			return fmt.Errorf("%w: symlinks unavailable: %v", godog.ErrSkip, err)
		}); err != nil {
			return err
		}
	case "python":
		if _, err := exec.LookPath("python3"); err != nil {
			return missing("python3")
		}
		for _, module := range []string{"pytest", "coverage"} {
			if exec.Command("python3", "-c", "import "+module).Run() != nil { //nolint:gosec // probe the installed python3 for a module this harness names
				return missing(module + " for python3")
			}
		}
		if strings.TrimSpace(c.IntegrationReport) != "" && exec.Command("python3", "-c", "import coverage; assert coverage.version_info[:2] >= (7, 13)").Run() != nil {
			return missing("coverage.py with subprocess support (7.13 or later)")
		}
		if c.Venv {
			if err := venv(dir, required); err != nil {
				return err
			}
		}
	case "kotlin":
		if _, err := exec.LookPath("java"); err != nil {
			return missing("java")
		}
	default:
		return fmt.Errorf("no tools known for %s", c.Language)
	}
	if c.Program != "" {
		if _, err := exec.LookPath(c.Program); err != nil {
			return missing(c.Program)
		}
	}
	return nil
}

// linkModules makes path a node_modules of links to modules' packages, its
// tools' caches left out (linkPackages), so the tools write those in the
// scenario's folder, and a link that fails as failedLink says: a
// node_modules there already linking modules' vitest is the one an earlier
// declaration made, as a Given that changes its project declares what the
// tools measure in it again.
func linkModules(modules, path string, failedLink func(error) error) error {
	if target, err := os.Readlink(filepath.Join(path, "vitest")); err == nil && target == filepath.Join(modules, "vitest") {
		return nil
	}
	return linkPackages(modules, path, nil, failedLink)
}

// missing is the error of a tool that is not installed: a skip, or a
// failure under ITOS_CC_LANGUAGE_TOOLS.
func missing(what string) error {
	if os.Getenv(languageToolsEnv) != "" {
		return fmt.Errorf("%s is not installed, and %s says the language tools are", what, languageToolsEnv)
	}
	return fmt.Errorf("%w: %s is not installed", godog.ErrSkip, what)
}

// languageJob skips a scenario needing language's tools in a CI job that
// did not install the pinned ones, for any language but Go: its runner's
// are whatever its image has.
func languageJob(language string) error {
	if language != "go" && os.Getenv(languageToolsEnv) == "" && os.Getenv("CI") != "" {
		return fmt.Errorf("%w: CI installs the %s tools on ubuntu-latest only, where it sets %s", godog.ErrSkip, language, languageToolsEnv)
	}
	return nil
}

// testTools are the programs each language's tests need, beside the one
// their command runs.
var testTools = map[string][]string{
	"go":         {"go"},
	"typescript": {"node"},
	"python":     {"python3"},
	"kotlin":     {"java"},
}

// Tests checks the tools t's tests run with are installed: the language's
// own, the program t's command runs when the PATH finds it, not a file of
// the project such as ./gradlew, and pytest for a command that runs it. A
// tool missing skips the scenario, or fails it under ITOS_CC_LANGUAGE_TOOLS,
// and any CI job but the one that installed the pinned tools skips a
// scenario needing other tools than Go's, as Coverage does. Tests said to
// run within quickRun skip where the system cannot run them that fast
// (quickRunsOn). The tests themselves then decide every outcome.
func (binary) Tests(_ string, t Tests) error {
	if t.Command == "" && t.Imports != nil {
		if t.Baseline != "" || len(t.Surviving) > 0 || len(t.TimingOut) > 0 || t.Elapsed != 0 || t.Trace != nil || t.Listed != nil {
			return fmt.Errorf("imports-only setup cannot declare execution outcomes")
		}
		return nil
	}
	tools, ok := testTools[t.Language]
	if !ok {
		return fmt.Errorf("no tools known for %s's tests", t.Language)
	}
	if t.Elapsed > 0 && t.Elapsed < quickRun && !quickRunsOn(runtime.GOOS) {
		return fmt.Errorf("%w: tests said to take %s run several times longer on %s, their first run most", godog.ErrSkip, t.Elapsed, runtime.GOOS)
	}
	if err := languageJob(t.Language); err != nil {
		return err
	}
	if words := strings.Fields(t.Command); len(words) > 0 && !strings.ContainsAny(words[0], `/\`) {
		tools = append(slices.Clone(tools), words[0])
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			return missing(tool)
		}
	}
	if strings.Contains(t.Command, "pytest") && exec.Command("python3", "-c", "import pytest").Run() != nil {
		return missing("pytest for python3")
	}
	return nil
}

// quickRun is the time under which tests run only as quickly as said on
// the systems quickRunsOn names.
const quickRun = 100 * time.Millisecond

// quickRunsOn is whether the system goos, as Go names it, runs a project's
// tests as quickly as a scenario says when that is under quickRun. On
// Windows a test command of 50 milliseconds took 0.2 to 0.5 seconds as a
// baseline, its trials 0.1 (CI run 38412153009), so a timeout its
// baseline sets cannot be told from the sentence's.
func quickRunsOn(goos string) bool { return goos != "windows" }

// Counted skips the scenario where itos-cc makes no counted run.
func (binary) Counted() error {
	if !CountedOn(runtime.GOOS) {
		return fmt.Errorf("%w: itos-cc makes counted runs on Linux and macOS alone (#29)", godog.ErrSkip)
	}
	return nil
}

// venv links python3 into dir as .venv/bin/python, the project's own
// virtualenv's python, with python3's pytest and coverage.
func venv(dir string, required bool) error {
	python, err := exec.LookPath("python3")
	if err == nil {
		python, err = filepath.Abs(python)
	}
	link := filepath.Join(dir, ".venv", "bin", "python")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(link), 0o755) //nolint:gosec // the scenario project's .venv/bin has the 0755 a real virtualenv's has; the product under test sees fixture modes, so narrowing them would change what scenarios check
	}
	if err != nil {
		return err
	}
	if err := os.Symlink(python, link); err != nil {
		if required {
			return err
		}
		return fmt.Errorf("%w: symlinks unavailable: %v", godog.ErrSkip, err)
	}
	return nil
}

// Link makes an owned fixture link; only typed capability refusals can omit.
func (b binary) Link(target, path string) error {
	if err := os.Symlink(target, path); err != nil {
		return coverageLinkFailure(b.Platform(), os.Getenv(languageToolsEnv) != "", err)
	}
	return nil
}

// nodeModules is the absolute node_modules holding vitest and
// @vitest/coverage-v8 that Coverage links, "" when there is none.
func (b binary) nodeModules() string {
	candidates := []string{
		filepath.Join(b.root, "testdata", "tools", "node", "node_modules"),
		filepath.Join(b.root, "viewer", "node_modules"),
	}
	if dir := os.Getenv(nodeModulesEnv); dir != "" {
		candidates = []string{dir}
	}
	for _, dir := range candidates {
		dir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if installed(dir, "vitest") && installed(dir, "@vitest/coverage-v8") {
			return dir
		}
	}
	return ""
}

// installed says whether modules holds the package name.
func installed(modules, name string) bool {
	_, err := os.Stat(filepath.Join(modules, filepath.FromSlash(name), "package.json")) //nolint:gosec // asks only whether a package the harness names is installed
	return err == nil
}

// Run runs the binary with r's arguments in r's folder, in the scenarios'
// environment with r's variables set and the programs r leaves out off its
// PATH, reading r's standard input.
func (b binary) Run(r Run) (Result, error) {
	bin := b.bin
	if r.Binary != "" {
		bin = r.Binary
	}
	cmd := exec.Command(bin, r.Args...) //nolint:gosec // the itos-cc binary under test, or a step's named one, with the scenario's arguments
	cmd.Dir = r.Dir
	cmd.Env = environment(r.Env)
	if len(r.Without) > 0 {
		cmd.Env = setEnv(cmd.Env, "PATH", pathHiding(append(slices.Clone(hiddenAlways), r.Without...)...))
	}
	if r.Stdin != "" {
		cmd.Stdin = strings.NewReader(r.Stdin)
	}
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	result := Result{Stdout: out.String(), Stderr: errOut.String()}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		result.Exit = exit.ExitCode()
	default:
		return result, fmt.Errorf("running %s: %w", bin, err)
	}
	return result, nil
}

// environment is the environment every command runs in, with each
// NAME=value of vars set, and each NAME of vars alone unset: the caller's, less what would make git, itos or
// itos-cc read anything but the scratch folder (a hook's GIT_DIR, the
// caller's ITOS_CC_ settings, CI's), with no global or system git config
// and a fixed identity, and the caller's PATH without its claude, its itos,
// its itos extensions or a git that is an itos (callerPath).
func environment(vars []string) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "GIT_") || strings.HasPrefix(name, "ITOS_") ||
			strings.HasPrefix(name, "ITOS_CC_") || strings.HasPrefix(name, "GITHUB_") ||
			strings.HasPrefix(name, "GH_") || name == "CI" || strings.EqualFold(name, "PATH") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=itos-cc features",
		"GIT_AUTHOR_EMAIL=features@localhost",
		"GIT_COMMITTER_NAME=itos-cc features",
		"GIT_COMMITTER_EMAIL=features@localhost",
		"PATH="+callerPath(),
	)
	for _, kv := range vars {
		name, value, set := strings.Cut(kv, "=")
		if !set {
			env = unsetEnv(env, name)
			continue
		}
		env = setEnv(env, name, value)
	}
	return env
}

// The programs no scenario reaches on the caller's PATH: its claude, its itos
// and its itos extensions (itos-*). An itos installed on the machine would
// run its own policy where a scenario meant only itos-cc (itos's
// T-087), and itos runs and lists every extension it finds, so one installed
// on the machine would change what a scenario sees (itos's T-080).
var hiddenAlways = []string{"claude", "itos", "itos-*"}

var (
	hiddenPathsMu sync.Mutex
	hiddenPaths   = map[string]string{}
	callerPathDir string
)

// callerPath is the caller's PATH with none of hiddenAlways on it, so no
// scenario reaches the Claude Code or the itos of the machine it runs on:
// each folder holding one is replaced by a folder of links to everything else
// in it, or left out where links cannot be made (windows without the right to
// make them).
func callerPath() string { return pathHiding(hiddenAlways...) }

// pathHiding is the caller's PATH with none of the programs named on it, nor
// a git that is an itos, made once a run for each set of names. A git shim of
// the caller's (itos git-shim install) would run the global itos for a
// scenario's git command; it is told from the real git by the rule itos
// itself goes by (isItos, git.go), so every git a scenario's commands
// start is the real one, as the steps' own are (realGit, itos's T-104).
func pathHiding(names ...string) string {
	hiddenPathsMu.Lock()
	defer hiddenPathsMu.Unlock()
	key := strings.Join(names, " ")
	if text, ok := hiddenPaths[key]; ok {
		return text
	}
	var folders []string
	for i, folder := range filepath.SplitList(os.Getenv("PATH")) {
		hidden := func(e os.DirEntry) bool {
			return isOneOf(e, names) || isOneOf(e, []string{"git"}) && isItos(filepath.Join(folder, e.Name()))
		}
		entries, err := os.ReadDir(folder)
		if err != nil || !slices.ContainsFunc(entries, hidden) {
			folders = append(folders, folder)
			continue
		}
		if callerPathDir == "" {
			if callerPathDir, err = os.MkdirTemp("", "itos-cc-features-path-"); err != nil {
				continue
			}
		}
		links := filepath.Join(callerPathDir, strconv.Itoa(len(hiddenPaths)), strconv.Itoa(i))
		if linkAllBut(folder, links, entries, hidden) == nil {
			folders = append(folders, links)
		}
	}
	text := strings.Join(folders, string(os.PathListSeparator))
	hiddenPaths[key] = text
	return text
}

// removeCallerPath removes the folders callerPath and pathHiding made.
func removeCallerPath() {
	if callerPathDir != "" {
		_ = os.RemoveAll(callerPathDir)
	}
}

// isOneOf is whether a folder's entry is a program of the names the PATH
// would find: the name, or on windows the name with any extension, in any
// case. A name ending in * is a prefix, so itos-* is every itos extension.
func isOneOf(e os.DirEntry, names []string) bool {
	name := e.Name()
	same := func(a, b string) bool { return a == b }
	starts := strings.HasPrefix
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
		same = strings.EqualFold
		starts = func(s, prefix string) bool {
			return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
		}
	}
	return slices.ContainsFunc(names, func(n string) bool {
		if prefix, ok := strings.CutSuffix(n, "*"); ok {
			return starts(name, prefix)
		}
		return same(name, n)
	})
}

// linkAllBut makes links a folder of links to every entry of folder but the
// hidden ones.
func linkAllBut(folder, links string, entries []os.DirEntry, hidden func(os.DirEntry) bool) error {
	if err := os.MkdirAll(links, 0o755); err != nil { //nolint:gosec // the folder of links standing for PATH has the 0755 a PATH folder has; the product under test sees fixture modes, so narrowing them would change what scenarios check
		return err
	}
	for _, e := range entries {
		if hidden(e) {
			continue
		}
		if err := os.Symlink(filepath.Join(folder, e.Name()), filepath.Join(links, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// realGit is the first git on the scenarios' PATH (callerPath) that is no
// itos, the one every command a scenario starts finds.
func realGit() (string, error) {
	for _, folder := range filepath.SplitList(callerPath()) {
		for _, name := range []string{"git.exe", "git"} {
			path := filepath.Join(folder, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() && !isItos(path) {
				return path, nil
			}
		}
	}
	return "", errors.New("no git on the PATH")
}
