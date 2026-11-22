package worktrees

import (
	"context"
	"slices"
	"testing"
)

func TestShellCommandPicksTheShellOfThePlatform(t *testing.T) {
	for _, c := range []struct {
		platform string
		want     []string
	}{
		{"windows", []string{"cmd", "/C", "echo hi"}},
		{"linux", []string{"sh", "-c", "echo hi"}},
	} {
		got := shellCommand(context.Background(), "echo hi", c.platform).Args
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.platform, got, c.want)
		}
	}
}
