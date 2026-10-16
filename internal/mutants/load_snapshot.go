package mutants

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/lang"
)

// LoadSnapshot admits a complete decoded record before observing current command
// policy. Missing or mismatched records never start a runner probe.
func LoadSnapshot(ctx context.Context, store MutationSnapshotReader, key string, tools Toolchain) (*Snapshot, error) {
	root, failure := store.Root(ctx)
	if failure != nil {
		return nil, failure
	}
	s, failure := store.Read(ctx, key)
	if failure != nil {
		return nil, failure
	}
	if s == nil || s.File != filepath.ToSlash(key) {
		return nil, nil
	}
	path := filepath.Join(root, filepath.FromSlash(key))
	if spec := lang.Detect(path); spec != nil && spec.Name == "typescript" {
		if tools == nil {
			return nil, InputFailed{Cause: errors.New("mutation Toolchain binding is missing")}
		}
		command, failure := ObserveCommand(ctx, tools, CommandRequest{Path: path})
		if failure != nil {
			return nil, failure
		}
		if ScopeForCommand("", false, command) == ScopeAllTests {
			for i := range s.Units {
				for j := range s.Units[i].Mutants {
					if m := &s.Units[i].Mutants[j]; m.TestScope() == ScopeOwn {
						m.Scope = ScopeAllTests
					}
				}
			}
		}
	}
	return s, nil
}

// ScopeForCommand retains the current broad fallback of built-in test scripts.
func ScopeForCommand(shell string, all bool, command Command) string {
	if scope := RunScope(shell, all); scope != ScopeOwn {
		return scope
	}
	if command.Suite {
		return ScopeAllTests
	}
	return ScopeOwn
}
