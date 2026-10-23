package worktrees

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// contractChild marks the child TestWorkersContract starts.
const contractChild = "T88_CONTRACT_CHILD"

// TestMain keeps a fixture child from running the package's tests. The
// tests start this binary again as a fixture, through the worker code under
// test, which runs it in a process group of its own; an environment variable
// says it is a child and -test.run selects its helper. A child whose
// arguments select no test, as when a mutant of how a worker passes them on
// puts the binary's own path before -test.run, would run every test again,
// each starting children of its own, without end, in groups a timed-out
// mutant's kill does not reach: nightly 39844317241 ended with 871 of them
// still running, and every mutant after them timed out (T-46). Such a child
// fails at once instead.
func TestMain(m *testing.M) {
	flag.Parse()
	child := os.Getenv(runnerFixtureMode) != "" || os.Getenv(contractChild) != ""
	if child && flag.Lookup("test.run").Value.String() == "" {
		_, _ = fmt.Fprintln(os.Stderr, "a fixture child selects no test: it runs none rather than every one")
		os.Exit(2)
	}
	os.Exit(m.Run())
}
