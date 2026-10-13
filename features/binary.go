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
)

// binary is the driver that runs itos-cc as another process, the binary at
// bin TestFeatures built from this tree, in folders of the system's
// temporary directory, with the real git: what proves main's wiring, the
// real adapters, exit codes and signals (decision 44).
type binary struct {
	bin string
}

var _ Driver = binary{}

// Folder makes a fresh folder in the system's temporary directory.
func (binary) Folder() (string, error) {
	return os.MkdirTemp("", "itos-cc-features-")
}

// Write writes data to path, making the folders above it.
func (binary) Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Read is the content of the file at path.
func (binary) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Mkdir makes the folder at path.
func (binary) Mkdir(path string) error {
	return os.Mkdir(path, 0o755)
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
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = environment(nil)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// Run runs the binary with r's arguments in r's folder, in the scenarios'
// environment with r's variables set, reading r's standard input.
func (b binary) Run(r Run) (Result, error) {
	cmd := exec.Command(b.bin, r.Args...)
	cmd.Dir = r.Dir
	cmd.Env = environment(r.Env)
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
		return result, fmt.Errorf("running %s: %w", b.bin, err)
	}
	return result, nil
}

// environment is the environment every command runs in, with each
// NAME=value of vars set: the caller's, less what would make git, itos or
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
		name, value, _ := strings.Cut(kv, "=")
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
	if err := os.MkdirAll(links, 0o755); err != nil {
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
