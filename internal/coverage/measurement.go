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

// Measurer measures coverage for a request, as the project's own tools do
// or as reports already written say: crap's, and mutation's through
// MutationMeasurer (decision 46). It is declared here, beside the Report it
// returns, so its implementations build that Report through the domain's
// API with no mapper between them (decision 47); its Request and Failure
// stay in port, which this package imports for Files and Gap. ctx carries
// only cancellation and deadlines, never a value (decision 30).
// porttest.MeasurerContract holds what every Measurer, the fake and
// internal/coveragetools', must do.
type Measurer interface {
	// Measure gives the report of req's Sources: each build root or named
	// report it could not measure is in the report's Missing with its Gap,
	// and what it measured of the rest is the report's. A Failure is a
	// measurement that gave no report at all: Cancelled once ctx is done,
	// whatever it measured before then kept by no one.
	Measure(ctx context.Context, req port.Request) (*Report, port.Failure)
}

// MutationMeasurer extends aggregate measurement with listed-test evidence and
// executable inventory. Model-bearing contracts live beside their values.
type MutationMeasurer interface {
	Measurer
	// MeasureListed measures req, attaching the listed tests only when fresh
	// built-in collectors run. Named/existing reports retain aggregate behavior.
	MeasureListed(ctx context.Context, req port.Request, tests *PerTest) (*Report, port.Failure)
	// Inventory lists executable lines of measured but unloaded sources without
	// running tests. Collector failures are logged and leave those files unproven;
	// cancellation returns Cancelled. The supplied Report retains good evidence.
	Inventory(ctx context.Context, report *Report, language string) port.Failure
}
