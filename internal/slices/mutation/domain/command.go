package domain

import "strings"

// Command runs the tests that should kill a file's mutants. Root is the tree a
// worker copies; Dir is where the command runs, inside Root.
type Command struct {
	Root  string
	Dir   string
	Args  []string
	Shell string // a user-supplied command line, instead of Args
	// PathEnv names an environment variable that gets the worker's copy of
	// these Root-relative directories prepended, so the copy wins over an
	// editable install that points at the real tree.
	PathEnv  string
	PathDirs []string
	// Suite says the command runs the whole suite of its build root, as no
	// narrower run exists: a TypeScript test script without Vitest or Jest.
	Suite bool
	// Ready, when set, is what a worker runs once in its copy of Root, in
	// Dir, before the copy's first command: flutter pub get --offline,
	// which resolves a Flutter package's copy from the pub cache alone, so
	// its tests run with --no-pub.
	Ready []string
}

// Key identifies a command for sharing one baseline between files.
func (c Command) Key() string {
	return c.Root + "\x00" + c.Dir + "\x00" + strings.Join(c.Args, "\x00") + c.Shell
}

func (c Command) String() string {
	if c.Shell != "" {
		return c.Shell
	}
	return strings.Join(c.Args, " ")
}

// RunsNothing says whether c runs no command at all: the own tests of a
// Python, Kotlin or Dart file no test reaches.
func (c Command) RunsNothing() bool {
	return len(c.Args) == 0 && c.Shell == ""
}
