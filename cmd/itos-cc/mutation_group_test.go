package main

import "testing"

// The scenarios of "Rule: The mutation group" in features/cli.feature run
// through godog (T-62); the helper the mutation tests share stays here.

// inEmptyDir makes an empty temporary directory the test's directory
// (useDir), so a command that mutates finds nothing to change.
func inEmptyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	useDir(t, dir)
	return dir
}
