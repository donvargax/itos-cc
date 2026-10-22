//go:build !windows

package processes

import "testing"

// TestNoMemoryCapHoldsOnMacOS passes macOS as the platform value, whose
// owned groups the runner kills and joins but whose memory it does not
// measure: a command holding more than the cap runs to its own end.
func TestNoMemoryCapHoldsOnMacOS(t *testing.T) {
	noMemoryCap(t, "darwin")
}
