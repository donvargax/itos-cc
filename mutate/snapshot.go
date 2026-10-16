package mutate

import (
	"context"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/internal/testtools"
)

// scopeCommand is the command that runs path's tests in scope, tests the
// test files that reach it.
func scopeCommand(ctx context.Context, path, scope string, tests []string) Command {
	switch scope {
	case ScopeOwn, ScopeListed, "":
		return TestCommand(ctx, path, "", false, tests)
	case ScopeAllTests:
		return TestCommand(ctx, path, "", true, nil)
	}
	return TestCommand(ctx, path, scope, false, nil)
}

// SnapshotName is where the snapshot of the file at rel, its path from the
// project root (project.FromRoot), lives under .metrics.
func SnapshotName(rel string) string {
	return filepath.Join("mutate", filepath.ToSlash(rel)+".json")
}

// LoadSnapshotOf reads the snapshot of the file at rel, its path from the
// root of the project whose snapshots store holds, which need not be the
// working directory's, or returns nil when there is none: the snapshot
// SnapshotName(rel), which holds while it names the file rel. One that
// names another file is none, so the file's functions are missing. Every
// reader looks a file's snapshot up here, mutation check and the
// architecture graph alike. A snapshot that cannot be read is the store's
// failure.
//
// An outcome it records as its file's own tests', where those run the whole
// suite (OwnScope), reads as ScopeAllTests, the scope that decided it: one
// recorded before that rule has no whole-suite evidence, so it reads stale
// once.
func LoadSnapshotOf(ctx context.Context, store snapport.Store, rel string, tools ...mutants.Toolchain) (*Snapshot, error) {
	var tool mutants.Toolchain = testtools.Toolchain{}
	if len(tools) > 0 && tools[0] != nil {
		tool = tools[0]
	}
	return mutants.LoadSnapshot(ctx, metricsdir.Mutations{Store: store}, rel, tool)
}

func scopeCommandWithTools(ctx context.Context, path, scope string, tests []string, tools mutants.Toolchain) (Command, error) {
	if tools == nil {
		return scopeCommand(ctx, path, scope, tests), nil
	}
	request := mutants.CommandRequest{Path: path, Tests: tests}
	switch scope {
	case ScopeOwn, ScopeListed, "":
	case ScopeAllTests:
		request.All = true
		request.Tests = nil
	default:
		request.Shell = scope
		request.Tests = nil
	}
	return mutants.ObserveCommand(ctx, tools, request)
}
