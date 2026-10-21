package porttest

import (
	"context"
	"testing"

	selport "github.com/donvargax/itos-cc/internal/selection/port"
	selporttest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// SettingsContract observes current semantic values, independent ownership and
// replacement. replace changes actual backing settings, not a call sequence.
// Codec validity and raw I/O failures remain adapter-owned tests. open's
// settings exclude generated/** alone, and replace's replaced/** alone: the
// Selection of every read keeps the selection engine's SettingsContract,
// which holds what a read gives of the exclude globs.
func SettingsContract(t *testing.T, open func(*testing.T) (mutants.SettingsReader, string, func())) {
	t.Helper()
	reader, root, replace := open(t)
	first, failure := reader.Read(t.Context())
	if failure != nil {
		t.Fatal(failure)
	}
	second, failure := reader.Read(t.Context())
	if failure != nil || second.Selection.Root != root || len(second.Exceptions) != 1 || second.Exceptions[0].Reason != "independent reason" || second.Tests == nil || len(second.Tests.Support) != 1 || second.Tests.Support[0] != "support.txt" {
		t.Fatalf("current semantic settings: %+v %v", second, failure)
	}
	first.Exceptions[0].Reason = "changed returned exception"
	first.Tests.Support[0] = "changed returned support"
	if second.Exceptions[0].Reason != "independent reason" || second.Tests.Support[0] != "support.txt" {
		t.Fatal("reads share mutable semantic settings")
	}
	replace()
	current, failure := reader.Read(t.Context())
	if failure != nil || current.Tests != nil || len(current.Exceptions) != 0 {
		t.Fatalf("backing replacement not reread: %+v %v", current, failure)
	}
	selporttest.SettingsContract(t, func(t *testing.T) (selport.SettingsReader, string, func()) {
		t.Helper()
		reader, root, replace := open(t)
		return selectionOf{reader}, root, replace
	})
}

// selectionOf is the Selection of each read of reader, the selection
// settings mutation reads with its own.
type selectionOf struct{ reader mutants.SettingsReader }

func (s selectionOf) ReadSelection(ctx context.Context) (selport.Settings, selport.SettingsFailure) {
	config, failure := s.reader.Read(ctx)
	if failure != nil {
		return selport.Settings{}, &selport.SettingsFailed{Cause: failure}
	}
	return config.Selection, nil
}
