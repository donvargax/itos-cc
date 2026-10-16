package mutants_test

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
)

func TestMemorySettingsContract(t *testing.T) {
	porttest.SettingsContract(t, func(t *testing.T) (mutants.SettingsReader, string, func()) {
		t.Helper()
		value := &porttest.Settings{Root: t.TempDir(), Value: mutants.Config{Exclude: []string{"generated/**"}, Exceptions: []mutants.Exception{{Reason: "independent reason"}}, Tests: &mutants.Tests{Support: []string{"support.txt"}}}}
		return value, value.Root, func() { value.Value = mutants.Config{Exclude: []string{"replaced/**"}} }
	})
}

func TestMemorySettingsCancellationRetainsCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("settings caller stopped")
	cancel(cause)
	reader := &porttest.Settings{}
	_, _, failure := reader.Read(ctx)
	var cancelled mutants.InputCancelled
	if !errors.As(failure, &cancelled) || !errors.Is(failure, cause) {
		t.Fatalf("caller cause: %v", failure)
	}
}
