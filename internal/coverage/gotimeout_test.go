package coverage_test

import (
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// go test prints each package's output whole, so a timeout's panic is
// followed by its own package's FAIL line: that pairs them. A package that
// failed otherwise, a test's own --- FAIL line and the closing FAIL are not
// timeouts.
func TestGoTimeoutsPairEachPanicWithItsPackage(t *testing.T) {
	output := "ok  \texample.com/m/fast\t0.1s\n" +
		"--- FAIL: TestBroken (0.00s)\nFAIL\nFAIL\texample.com/m/broken\t0.2s\n" +
		"panic: test timed out after 10m0s\r\n\trunning tests:\n\t\tTestSlow (10m0s)\n\ngoroutine 7 [running]:\n" +
		"FAIL\texample.com/m/slow\t600.1s\n" +
		"panic: test timed out after 1s \nFAIL\texample.com/m/slower\t1.0s\nFAIL\n"
	want := []coverage.GoTimeout{{Package: "example.com/m/slow", Limit: "10m0s"}, {Package: "example.com/m/slower", Limit: "1s"}}
	if got := coverage.ReadGoTimeouts(output); !slices.Equal(got, want) {
		t.Errorf("ReadGoTimeouts = %v, want %v", got, want)
	}
	if got := coverage.ReadGoTimeouts("panic: test timed out after 1s\n"); len(got) != 0 {
		t.Errorf("a panic with no FAIL line after it: %v, want none", got)
	}
}

// An Unmeasured names each package that timed out after its reason, and is
// marked so; with none it is as it was.
func TestAnUnmeasuredNamesThePackagesThatTimedOut(t *testing.T) {
	u := coverage.Unmeasured{Dir: "slow", Language: "go", Cause: port.MeasuredNothing{}, Reason: "go: exit status 1"}
	if same := u.WithTimeouts(nil); same != u {
		t.Errorf("with no timeout: %+v, want %+v", same, u)
	}
	timed := u.WithTimeouts([]coverage.GoTimeout{{Package: "example.com/a", Limit: "1s"}, {Package: "example.com/b", Limit: "2s"}})
	want := "slow (go): go: exit status 1" +
		"; example.com/a timed out after 1s, go test's limit per package: raise it with GOFLAGS=-timeout=<duration>" +
		"; example.com/b timed out after 2s, go test's limit per package: raise it with GOFLAGS=-timeout=<duration>"
	if !timed.TimedOut || timed.String() != want {
		t.Errorf("with two: %q, timed out %v; want %q", timed.String(), timed.TimedOut, want)
	}
}
