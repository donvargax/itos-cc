package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// slowRun is a run of "The baseline proves the copy works" on a slow
// Windows runner, as CD run 39163467526 made it: itos-cc measures a
// baseline of 4.1s, most of it building the tests in a fresh copy, while
// the mutants after it find them built, and times out the mutants of line
// 7 after ten times that, the allowance, and the second its process tree
// takes to end. Its tests' records are the times of a real run, from the
// moment it is launched.
type slowRun struct {
	binary
	records string
	// timeout is how long the mutants of line 7 ran before they timed out.
	timeout float64
}

func (slowRun) Tests(string, Tests) error { return nil }

func (slowRun) Platform() string { return "windows" }

// Run records the tests' runs, each a [start, end] of its test in seconds
// after the launch, the baseline's first, and says what the run said.
func (s slowRun) Run(Run) (Result, error) {
	launched := time.Now()
	at := func(seconds float64) int64 {
		return launched.Add(time.Duration(seconds * float64(time.Second))).UnixMilli()
	}
	var lines []string
	for i, test := range [][2]float64{{3.7, 4.2}, {5.6, 6.1}, {5.7, 6.2}, {7.4, 7.9}} {
		data, err := json.Marshal(testRecord{Cwd: filepath.Join("copy", fmt.Sprint(i%2), "src"), Start: at(test[0]), End: at(test[1]), Mutated: i > 0})
		if err != nil {
			return Result{}, err
		}
		lines = append(lines, string(data))
	}
	if err := s.Write(filepath.Join(s.records, "runs"), []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return Result{}, err
	}
	stderr := "itos-cc: baseline .$ go test -count=1 -failfast ./src\n" +
		"itos-cc: [1/5] src/board.go:6 `>` → `>=` killed (2.4s)\n" +
		"itos-cc: [2/5] src/board.go:5 `0` → `1` killed (2.4s)\n" +
		"itos-cc: [3/5] src/board.go:6 `0` → `1` killed (1.4s)\n" +
		fmt.Sprintf("itos-cc: [4/5] src/board.go:7 `-` → `+` timeout (%.1fs)\n", s.timeout) +
		fmt.Sprintf("itos-cc: [5/5] src/board.go:7 `1` → `0` timeout (%.1fs)\n", s.timeout)
	return Result{Stdout: "src/board.go: 5 killed, 0 survived, 0 uncovered (ran 5, reused 0)\n", Stderr: stderr}, nil
}

// The timeout of a slow but correct run is the one its baseline sets, and
// one not set from it, fixed beyond what any baseline the run measured
// sets, is still refused.
func TestBaselineSetsTimeoutOfASlowRun(t *testing.T) {
	cases := []struct {
		name    string
		timeout float64
		holds   bool
	}{
		{"set from the baseline", 46.9, true},
		{"fixed past it", 90, false},
		{"fixed short of it", 8, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			records := t.TempDir()
			m := &mutateState{w: &world{d: slowRun{records: records, timeout: c.timeout}, dir: t.TempDir()}, records: records}
			if err := m.aboutToRun(); err != nil {
				t.Fatal(err)
			}
			err := m.durationSetsTimeout()
			if c.holds && err != nil {
				t.Fatalf("refused the timeout its baseline sets: %v", err)
			}
			if !c.holds && err == nil {
				t.Fatalf("held a timeout of %gs the baseline does not set", c.timeout)
			}
		})
	}
}
