package project

import (
	"slices"
	"strings"
	"testing"
)

// A GOCOVERDIR set for itos-cc never reaches a command it starts, while
// the rest of its environment does, and a coverage run of its own sets one
// on top (slice-59, q-96). ID-MUT-352 holds the same through the built
// binary; this holds the environment builder where ci.yml runs no binary
// driver. The process's own variable stays, so measuring itos-cc works.
func TestInheritedEnvLeavesOutGOCOVERDIR(t *testing.T) {
	t.Setenv("GOCOVERDIR", t.TempDir())
	t.Setenv("ITOS_CC_ENV_KEPT", "kept")

	for name, env := range map[string][]string{
		"InheritedEnv":       InheritedEnv(),
		"NoBytecodeEnv(nil)": NoBytecodeEnv(nil),
	} {
		if dir, ok := lookup(env, "GOCOVERDIR"); ok {
			t.Errorf("%s holds GOCOVERDIR=%s", name, dir)
		}
		if kept, _ := lookup(env, "ITOS_CC_ENV_KEPT"); kept != "kept" {
			t.Errorf("%s holds ITOS_CC_ENV_KEPT=%q, not %q", name, kept, "kept")
		}
	}
	if bytecode, _ := lookup(NoBytecodeEnv(nil), "PYTHONDONTWRITEBYTECODE"); bytecode != "1" {
		t.Errorf("NoBytecodeEnv(nil) holds PYTHONDONTWRITEBYTECODE=%q, not 1", bytecode)
	}

	own := t.TempDir()
	if dir, _ := lookup(NoBytecodeEnv(append(InheritedEnv(), "GOCOVERDIR="+own)), "GOCOVERDIR"); dir != own {
		t.Errorf("an own coverage run's environment holds GOCOVERDIR=%q, not %q", dir, own)
	}
	if got := gitCommand("", "version").Env; !slices.Equal(got, InheritedEnv()) {
		t.Errorf("git runs in %q, not InheritedEnv", got)
	}
}

// A command that starts in a directory of its own gets PWD naming it, as
// os/exec gives one whose environment it builds itself, so Go's tools print
// the directory in the form it was given, a symbolic link unresolved
// (bug-10). One that starts in the working directory keeps InheritedEnv.
func TestInheritedEnvInNamesTheDirectoryACommandStartsIn(t *testing.T) {
	t.Setenv("GOCOVERDIR", t.TempDir())
	t.Setenv("PWD", t.TempDir())
	dir := t.TempDir()

	env := InheritedEnvIn(dir)
	if pwd, _ := lookup(env, "PWD"); pwd != dir {
		t.Errorf("InheritedEnvIn(%s) holds PWD=%s", dir, pwd)
	}
	if got, ok := lookup(env, "GOCOVERDIR"); ok {
		t.Errorf("InheritedEnvIn holds GOCOVERDIR=%s", got)
	}
	if got := InheritedEnvIn(""); !slices.Equal(got, InheritedEnv()) {
		t.Errorf("InheritedEnvIn(\"\") is %q, not InheritedEnv", got)
	}
	if cmd := gitCommand(dir, "version"); cmd.Dir != dir || !slices.Equal(cmd.Env, env) {
		t.Errorf("git starts in %q with %q, want %s with InheritedEnvIn", cmd.Dir, cmd.Env, dir)
	}
	if cmd := historyCommand(t.Context(), dir, "version"); cmd.Dir != dir || !slices.Equal(cmd.Env, env) {
		t.Errorf("history's git starts in %q with %q, want %s with InheritedEnvIn", cmd.Dir, cmd.Env, dir)
	}
}

// lookup is the last value env gives name, as exec.Cmd takes it.
func lookup(env []string, name string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if n, v, _ := strings.Cut(kv, "="); n == name {
			value, found = v, true
		}
	}
	return value, found
}
