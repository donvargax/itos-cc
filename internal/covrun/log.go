package covrun

import (
	"fmt"
	"io"
	"time"
)

// logf writes a line of a coverage run's log, where the tools' own output
// goes too. What is measured and returned never depends on the log taking
// the line, so a write it refuses is not the run's error and is dropped
// here, once, rather than at every line.
func logf(log io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(log, format, args...)
}

// listedRan says the listed tests' coverage run ended, after took, and
// that the coverage of n of them, the tests that wrote data, is converted
// next: the start of the conversions, which say nothing else until they
// end. alone is whether each test ran alone, as the harness did not split
// the run of every test.
func listedRan(log io.Writer, alone bool, took time.Duration, n int) {
	ran := "ran"
	if alone {
		ran = "ran one at a time"
	}
	logf(log, "itos-cc: coverage: the listed tests %s in %.1fs; converting the coverage of %d of them\n", ran, took.Seconds(), n)
}

// listedConverted says the conversions of the coverage of n listed tests
// ended, after took.
func listedConverted(log io.Writer, n int, took time.Duration) {
	logf(log, "itos-cc: coverage: converted the coverage of %d listed tests in %.1fs\n", n, took.Seconds())
}
