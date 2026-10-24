package mutation

import (
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/cli"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// An itos-cc.yaml refused with a fix of its own, as an anchor is (bug-8),
// is config.invalid with that fix, wherever it was read: as the settings,
// as a counted run's plan, or by the writer of mutation.exceptions. One
// refused without keeps the fix each reader gives.
func TestAConfigRefusalKeepsItsOwnFix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name             string
		withOwn, without error
		shared           string
	}{
		{"settings", settingsError(mutants.SettingsInvalid{Reason: "r", Key: "x", Fix: "own"}), settingsError(mutants.SettingsInvalid{Reason: "r", Key: "x"}), "Fix it: exclude lists"},
		{"counted plan", countedPlanError(mutants.FreshInvalid{Reason: "r", Key: "x", Fix: "own"}, ""), countedPlanError(mutants.FreshInvalid{Reason: "r", Key: "x"}, ""), "Fix it in a commit"},
		{"writer", updateError(mutants.ExceptionInvalid{Reason: "r", Key: "x", Fix: "own"}, "caller's"), updateError(mutants.ExceptionInvalid{Reason: "r", Key: "x"}, "caller's"), "caller's"},
	} {
		var p *cli.Problem
		if !errors.As(c.withOwn, &p) || p.Rule != "config.invalid" || p.Fix != "own" || p.Subject("key") != "x" || p.Message != "itos-cc.yaml: r" {
			t.Errorf("%s with its own fix: %+v", c.name, c.withOwn)
		}
		if !errors.As(c.without, &p) || !strings.HasPrefix(p.Fix, c.shared) {
			t.Errorf("%s without: %+v, want a fix starting %q", c.name, c.without, c.shared)
		}
	}
}
