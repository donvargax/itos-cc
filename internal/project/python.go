package project

import "os"

// NoBytecodeEnv is env, or the parent's environment when env is nil, with
// Python told to write no bytecode. Python takes the bytecode in
// __pycache__ as fresh while its source keeps the mtime, in whole seconds,
// and the size it was compiled from, so a mutant of the same size written
// within the second a run compiled its source would run the unmutated
// code. Every test and coverage command itos-cc runs gets it: one that
// writes no bytecode can leave none stale, and leaves no __pycache__ in the
// project's own tree. A command that is not Python ignores it, and a shell
// line may start Python without itos-cc knowing.
func NoBytecodeEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	return append(env[:len(env):len(env)], "PYTHONDONTWRITEBYTECODE=1")
}
