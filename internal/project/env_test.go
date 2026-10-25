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
	if got := gitCommand("version").Env; !slices.Equal(got, InheritedEnv()) {
		t.Errorf("git runs in %q, not InheritedEnv", got)
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
