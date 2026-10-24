package coverage

import (
	"fmt"
	"strings"
)

// GoTimeout is a Go package whose test binary go test stopped at its
// -timeout, 10 minutes per package unless GOFLAGS or the command raises it:
// its coverage was cut short, and the limit is what to raise (slice-47).
type GoTimeout struct {
	Package string
	Limit   string
}

// String names the package and its limit, and how to raise the limit:
// itos-cc passes go test no -timeout of its own, so GOFLAGS sets it.
func (t GoTimeout) String() string {
	return fmt.Sprintf("%s timed out after %s, go test's limit per package: raise it with GOFLAGS=-timeout=<duration>", t.Package, t.Limit)
}

const goTimeoutPanic = "panic: test timed out after "

// GoTimeouts reads go test's output, a line at a time, for the packages
// whose tests timed out: a test binary that outlives its limit prints
// "panic: test timed out after <limit>", and go test then ends that
// package's output with its "FAIL\t<package>\t..." line, as it prints each
// package's output whole. The zero value is ready to read.
type GoTimeouts struct {
	limit string
	found []GoTimeout
}

// Line reads one line of the output, its line ending trimmed or not.
func (g *GoTimeouts) Line(line string) {
	line = strings.TrimRight(line, "\r\n")
	if limit, ok := strings.CutPrefix(line, goTimeoutPanic); ok {
		g.limit = strings.TrimSpace(limit)
		return
	}
	if g.limit == "" {
		return
	}
	if fields := strings.Split(line, "\t"); len(fields) >= 2 && fields[0] == "FAIL" {
		g.found = append(g.found, GoTimeout{Package: fields[1], Limit: g.limit})
		g.limit = ""
	}
}

// Found is each package the lines read named as timed out, in the order
// go test printed them.
func (g *GoTimeouts) Found() []GoTimeout { return g.found }

// ReadGoTimeouts is the packages output, go test's whole output, names as
// timed out (GoTimeouts).
func ReadGoTimeouts(output string) []GoTimeout {
	var g GoTimeouts
	for line := range strings.Lines(output) {
		g.Line(line)
	}
	return g.Found()
}

// WithTimeouts is u with each of timeouts named after its reason, and marked
// TimedOut when there is one; u as it was when there is none.
func (u Unmeasured) WithTimeouts(timeouts []GoTimeout) Unmeasured {
	for _, t := range timeouts {
		u.Reason += "; " + t.String()
		u.TimedOut = true
	}
	return u
}
