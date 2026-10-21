package porttest

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/selection/port"
)

// The fake holds the current selection settings as config.Reader reads
// them from itos-cc.yaml.
func TestSettingsKeepsTheContract(t *testing.T) {
	SettingsContract(t, func(t *testing.T) (port.SettingsReader, string, func()) {
		t.Helper()
		value := &Settings{Value: port.Settings{Root: t.TempDir(), Exclude: []string{"generated/**"}}}
		return value, value.Value.Root, func() { value.Value = port.Settings{Root: value.Value.Root, Exclude: []string{"replaced/**"}} }
	})
}

// A done context is Cancelled, keeping its cause, and a configured failure
// is the read's.
func TestSettingsFailsAsConfigured(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("settings caller stopped")
	cancel(cause)
	var cancelled *port.Cancelled
	if _, failure := (&Settings{}).ReadSelection(ctx); !errors.As(failure, &cancelled) || !errors.Is(failure, cause) {
		t.Fatalf("caller cause: %v", failure)
	}
	invalid := &port.SettingsInvalid{Reason: `exclude[0] "" is empty`, Key: "exclude[0]"}
	if value, failure := (&Settings{Value: port.Settings{Root: "/work"}, Failure: invalid}).ReadSelection(t.Context()); failure != invalid || value.Root != "" {
		t.Fatalf("configured failure: %+v %v", value, failure)
	}
}
