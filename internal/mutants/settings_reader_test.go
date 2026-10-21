package mutants_test

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
)

func TestMemorySettingsContract(t *testing.T) {
	porttest.SettingsContract(t, func(t *testing.T) (mutants.SettingsReader, string, func()) {
		t.Helper()
		root := t.TempDir()
		value := &porttest.Settings{Value: mutants.Config{Selection: selport.Settings{Root: root, Exclude: []string{"generated/**"}}, Exceptions: []mutants.Exception{{Reason: "independent reason"}}, Tests: &mutants.Tests{Support: []string{"support.txt"}}}}
		return value, root, func() {
			value.Value = mutants.Config{Selection: selport.Settings{Root: root, Exclude: []string{"replaced/**"}}}
		}
	})
}

func TestMemorySettingsCancellationRetainsCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("settings caller stopped")
	cancel(cause)
	reader := &porttest.Settings{}
	_, failure := reader.Read(ctx)
	var cancelled mutants.InputCancelled
	if !errors.As(failure, &cancelled) || !errors.Is(failure, cause) {
		t.Fatalf("caller cause: %v", failure)
	}
}
