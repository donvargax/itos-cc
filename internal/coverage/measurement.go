package coverage

import (
	"context"

	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// Test is one test a project lists: its ID, and the file that defines it as
// the list command names it, or "".
type Test struct {
	ID, File string
}

// PerTest is how to run a project's listed tests: shell command lines run
// through the platform shell in Root.
type PerTest struct {
	Root  string
	Tests []Test
	// All runs every test.
	All string
	// Select runs the tests ids.
	Select func(ids []string) string
}

// MutationMeasurer extends aggregate measurement with listed-test evidence and
// executable inventory. Model-bearing contracts live beside their values.
type MutationMeasurer interface {
	port.Measurer[*Report]
	// MeasureListed measures req, attaching the listed tests only when fresh
	// built-in collectors run. Named/existing reports retain aggregate behavior.
	MeasureListed(ctx context.Context, req port.Request, tests *PerTest) (*Report, port.Failure)
	// Inventory lists executable lines of measured but unloaded sources without
	// running tests. Collector failures are logged and leave those files unproven;
	// cancellation returns Cancelled. The supplied Report retains good evidence.
	Inventory(ctx context.Context, report *Report, language string) port.Failure
}
