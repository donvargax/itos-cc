package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The repository's README.md and docs/CLI.md describe counted mode's and
// --fail-fast's platforms, and --fail-fast's rule and output, as the help
// does, which ID-MUT-191 and 199 read. Kept: no scenario reads the
// repository's documentation, so these documentation assertions, split
// from the ID-MUT-191 and 199 holders, stay beside the command they
// document, read without running it (T-130).
func TestTheDocsDescribeCountedAndFailFastPlatforms(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"README.md", filepath.Join("docs", "CLI.md")} {
		data, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		// Linux and macOS are counted mode's and --fail-fast's platforms,
		// and Windows is #29.
		for _, want := range []string{"Linux and macOS", "#29", "--fail-fast", "fail-fast.platform", "unattempted"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not describe %s", name, want)
			}
		}
		if strings.Contains(text, "Linux only") || strings.Contains(text, "Linux-only") {
			t.Errorf("%s still says counted mode is Linux only", name)
		}
		// The stop object, its key quoted as JSON or as code.
		if !strings.Contains(text, `"stop"`) && !strings.Contains(text, "`stop`") {
			t.Errorf("%s does not describe the stop object", name)
		}
	}
}
