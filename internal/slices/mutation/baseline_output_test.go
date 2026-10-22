package mutation

import (
	"fmt"
	"strings"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// TestAFailingBaselineReportsTheHeadAndTailOfItsOutput holds issue #47's
// report: whole up to 40 lines, else the first 20 and the last 20 with a
// line saying how many were left out between, those the run did not keep
// among them. MUT-303 holds 100 lines through the command; an output cut
// past 128 KiB, and the counts at 40 and 41 lines, are held here, as no
// scenario writes one of each.
func TestAFailingBaselineReportsTheHeadAndTailOfItsOutput(t *testing.T) {
	numbered := func(from, to int) []string {
		var lines []string
		for i := from; i <= to; i++ {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		return lines
	}
	joined := func(parts ...[]string) string {
		var all []string
		for _, p := range parts {
			all = append(all, p...)
		}
		return strings.Join(all, "\n")
	}
	if got, want := HeadAndTail(mutants.Output{Text: joined(numbered(1, 40)) + "\n\n"}, 20), joined(numbered(1, 40)); got != want {
		t.Errorf("40 lines reported as\n%s", got)
	}
	if got, want := HeadAndTail(mutants.Output{Text: joined(numbered(1, 100)) + "\n"}, 20), joined(numbered(1, 20), []string{"… 60 lines left out …"}, numbered(81, 100)); got != want {
		t.Errorf("100 lines reported as\n%s", got)
	}
	if got, want := HeadAndTail(mutants.Output{Text: joined(numbered(1, 41))}, 20), joined(numbered(1, 20), []string{"… 1 line left out …"}, numbered(22, 41)); got != want {
		t.Errorf("41 lines reported as\n%s", got)
	}
	// Kept as a run keeps a long output: its first lines 1 to 30, its last
	// 971 to 1000, the 940 between left out.
	head, tail := joined(numbered(1, 30))+"\n", joined(numbered(971, 1000))+"\n"
	cut := mutants.Output{Text: head + tail, Cut: true, At: len(head), LeftOut: 940}
	if got, want := HeadAndTail(cut, 20), joined(numbered(1, 20), []string{"… 960 lines left out …"}, numbered(981, 1000)); got != want {
		t.Errorf("a cut output reported as\n%s", got)
	}
}
