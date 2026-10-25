package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// coverDirVariable tells a program built with -cover where to write its
// coverage counters.
const coverDirVariable = "GOCOVERDIR"

// InheritedEnv is itos-cc's own environment as every command it starts
// inherits it: without GOCOVERDIR, its name's case ignored as Windows
// ignores it (slice-59, q-96). A GOCOVERDIR set for itos-cc says where
// itos-cc's own counters go, when it is built with -cover; passed on, the
// project's tests run without coverage would write counters where nobody
// asked, and read it as a coverage run. itos-cc's own process keeps it, so
// measuring itos-cc still works, and the coverage runs itos-cc makes
// itself set their own on top of this. Every command itos-cc starts begins
// from it, alone or through NoBytecodeEnv.
func InheritedEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if name, _, _ := strings.Cut(kv, "="); !strings.EqualFold(name, coverDirVariable) {
			out = append(out, kv)
		}
	}
	return out
}

// InheritedEnvIn is InheritedEnv for a command that starts in dir, with
// PWD naming dir, as os/exec names it when it builds a command's
// environment itself, and only then. Go's own tools take PWD as their
// working directory when it names the directory they run in, so a dir
// reached through a symbolic link, as macOS's temporary folder /var is
// /private/var, keeps in what they print the form the caller gave it:
// without it, go list printed physical directories that matched no source
// and a coverage run measured the whole module (bug-10). An empty dir is
// the working directory, whose PWD InheritedEnv already holds.
func InheritedEnvIn(dir string) []string {
	env := InheritedEnv()
	if abs, err := filepath.Abs(dir); dir != "" && err == nil {
		env = append(env, "PWD="+abs)
	}
	return env
}

// NoBytecodeEnv is env, or InheritedEnv when env is nil, with Python told
// to write no bytecode. Python takes the bytecode in __pycache__ as fresh
// while its source keeps the mtime, in whole seconds, and the size it was
// compiled from, so a mutant of the same size written within the second a
// run compiled its source would run the unmutated code. Every test and
// coverage command itos-cc runs gets it: one that writes no bytecode can
// leave none stale, and leaves no __pycache__ in the project's own tree. A
// command that is not Python ignores it, and a shell line may start Python
// without itos-cc knowing.
func NoBytecodeEnv(env []string) []string {
	if env == nil {
		env = InheritedEnv()
	}
	return append(env[:len(env):len(env)], "PYTHONDONTWRITEBYTECODE=1")
}

// gitCommand is git with args, started in dir, the working directory when
// empty, in InheritedEnvIn.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...) //nolint:gosec // only the fixed git program, with the arguments this package builds
	cmd.Dir, cmd.Env = dir, InheritedEnvIn(dir)
	return cmd
}
