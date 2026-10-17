package main

import (
	"path/filepath"
	"testing"
)

// Helpers other tests share, over the module of mutate_since_test.go. The
// tests of "Rule: Results go stale when their tests change" that were here
// went once its scenarios, ID-MUT-65 to 70, held each assertion (T-101).

// changeBoardTest changes src/board_test.go without changing what it tests.
func changeBoardTest(t *testing.T) {
	t.Helper()
	appendTo(t, filepath.FromSlash("src/board_test.go"), "\n// TestClear checks clear.\n")
}
