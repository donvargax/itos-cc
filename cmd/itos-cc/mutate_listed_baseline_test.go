package main

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Helpers the counted tests share, from the tests of "Rule: Each selection
// of listed tests has its own baseline" in features/mutate.feature, which
// ID-MUT-135 to 137 hold whole (T-101). They use the module of
// mutate_listed_test.go, whose harness also records, on each run, the
// SHA-256 of the main.go it builds the binary from: a run whose hash is that
// of the main.go written ran without a mutant. ID-A-01 alone reaches the
// lines of greet's n == 1 branch, ID-A-02 alone those of n-1 (line 17), and
// both the rest of greet and main.

// The harness's run record, and what it becomes: the same line, with the
// hash of ../main.go.
const (
	harnessRecord   = `fmt.Fprintf(f, "run tests=%s coverdir=%s\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"))`
	harnessRecorded = `src, _ := os.ReadFile(filepath.Join("..", "main.go"))
		fmt.Fprintf(f, "run tests=%s coverdir=%s main=%x\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"), sha256.Sum256(src))`
)

// selectionRepo makes the module of mutate_listed_test.go, with the harness
// recording main.go's hash and each of changes made, the first of its
// text in the file it names, as listedRepo does. It returns the record file
// and the text of main.go.
func selectionRepo(t *testing.T, changes ...[3]string) (string, string) {
	t.Helper()
	files := map[string]string{}
	for name, text := range listedFiles {
		files[name] = text
	}
	changes = append([][3]string{
		{"e2e/e2e_test.go", `import (` + "\n", "import (\n\t\"crypto/sha256\"\n"},
		{"e2e/e2e_test.go", harnessRecord, harnessRecorded},
	}, changes...)
	for _, c := range changes {
		if !strings.Contains(files[c[0]], c[1]) {
			t.Fatalf("%s holds no %q", c[0], c[1])
		}
		files[c[0]] = strings.Replace(files[c[0]], c[1], c[2], 1)
	}
	moduleRepo(t, files)
	record := filepath.Join(t.TempDir(), "record")
	useEnv(t, "GREET_RECORD", record)
	return record, files["main.go"]
}

// unmutatedRuns is how many runs of record's harness selected ids alone,
// with no ITOS_CC_TEST_COVERDIR, on a main.go that is main: without a
// mutant, outside coverage. It also returns the index of each such run and
// of the first run selecting ids on any other main.go, -1 when none did.
func unmutatedRuns(lines []string, ids, main string) (int, []int, int) {
	prefix := "run tests=" + ids + " coverdir= main="
	clean := prefix + fmt.Sprintf("%x", sha256.Sum256([]byte(main)))
	var at []int
	mutated := -1
	for i, l := range lines {
		switch {
		case l == clean:
			at = append(at, i)
		case strings.HasPrefix(l, prefix) && mutated < 0:
			mutated = i
		}
	}
	return len(at), at, mutated
}
