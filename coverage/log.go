package coverage

import (
	"fmt"
	"io"
)

// logf writes a line of a coverage run's log, where the tools' own output
// goes too. What is measured and returned never depends on the log taking
// the line, so a write it refuses is not the run's error and is dropped
// here, once, rather than at every line.
func logf(log io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(log, format, args...)
}
