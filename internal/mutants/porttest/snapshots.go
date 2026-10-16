package porttest

import (
	"context"
	"maps"
	"slices"

	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Snapshots holds independent model values; encoding is exclusively concrete.
// Reads/writes own their nested values so later fixture edits cannot change history.
type Snapshots struct {
	RootDir                    string
	Values                     map[string]mutants.Snapshot
	Failures                   map[string]snapport.Failure
	WriteFailures              map[string]snapport.Failure
	RootFailure                snapport.Failure
	ReadKeys, Written, Removed []string
}

func (s *Snapshots) Root(ctx context.Context) (string, snapport.Failure) {
	if ctx.Err() != nil {
		return "", &snapport.Cancelled{}
	}
	return s.RootDir, s.RootFailure
}
func (s *Snapshots) Read(ctx context.Context, key string) (*mutants.Snapshot, snapport.Failure) {
	if ctx.Err() != nil {
		return nil, &snapport.Cancelled{}
	}
	if failure := s.Failures[key]; failure != nil {
		return nil, failure
	}
	s.ReadKeys = append(s.ReadKeys, key)
	value, ok := s.Values[key]
	if !ok {
		return nil, nil
	}
	value = cloneSnapshot(value)
	return &value, nil
}
func (s *Snapshots) Write(ctx context.Context, key string, value mutants.Snapshot) snapport.Failure {
	if ctx.Err() != nil {
		return &snapport.Cancelled{}
	}
	if failure := s.WriteFailures[key]; failure != nil {
		return failure
	}
	if s.Values == nil {
		s.Values = map[string]mutants.Snapshot{}
	}
	s.Values[key] = cloneSnapshot(value)
	s.Written = append(s.Written, key)
	return nil
}
func (s *Snapshots) Remove(ctx context.Context, key string) snapport.Failure {
	if ctx.Err() != nil {
		return &snapport.Cancelled{}
	}
	if failure := s.Failures[key]; failure != nil {
		return failure
	}
	delete(s.Values, key)
	s.Removed = append(s.Removed, key)
	return nil
}
func cloneSnapshot(value mutants.Snapshot) mutants.Snapshot {
	value.Tests = maps.Clone(value.Tests)
	value.Listed = maps.Clone(value.Listed)
	value.ListedFiles = maps.Clone(value.ListedFiles)
	value.Support = maps.Clone(value.Support)
	value.Units = slices.Clone(value.Units)
	for i := range value.Units {
		u := &value.Units[i]
		u.Mutants = slices.Clone(u.Mutants)
		if u.Coverage != nil {
			c := *u.Coverage
			c.Inputs = maps.Clone(c.Inputs)
			c.Blocks = slices.Clone(c.Blocks)
			u.Coverage = &c
		}
		for j := range u.Mutants {
			m := &u.Mutants[j]
			m.Tests = slices.Clone(m.Tests)
			if m.SuiteEvidence != nil {
				e := *m.SuiteEvidence
				e.Tests = maps.Clone(e.Tests)
				e.Support = maps.Clone(e.Support)
				m.SuiteEvidence = &e
			}
		}
	}
	return value
}
