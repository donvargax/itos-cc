package porttest

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/mutants"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Snapshots holds independent model values; encoding is exclusively concrete.
// Reads/writes own their nested values so later fixture edits cannot change history.
type Snapshots struct {
	RootDir       string
	Values        map[string]mutants.Snapshot
	Failures      map[string]snapport.Failure
	WriteFailures map[string]snapport.Failure
	RootFailure   snapport.Failure
	// CatalogFiles are independent permissive metadata, keyed by store-relative names.
	CatalogFiles               map[string]string
	CatalogFailures            map[string]snapport.Failure
	ListFailure                snapport.Failure
	MetadataReads              []string
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
	name := mutationCatalogName(key)
	if s.CatalogFiles == nil {
		s.CatalogFiles = map[string]string{}
	}
	s.CatalogFiles[name] = value.File
	delete(s.CatalogFailures, name)
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
	name := mutationCatalogName(key)
	delete(s.CatalogFiles, name)
	delete(s.CatalogFailures, name)
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

func (s *Snapshots) List(ctx context.Context) ([]string, snapport.Failure) {
	if ctx.Err() != nil {
		return nil, &snapport.Cancelled{}
	}
	if s.ListFailure != nil {
		return nil, s.ListFailure
	}
	set := map[string]bool{}
	for key := range s.Values {
		set[mutationCatalogName(key)] = true
	}
	for name := range s.CatalogFiles {
		if strings.HasPrefix(name, "mutate/") && strings.HasSuffix(name, ".json") {
			set[name] = true
		}
	}
	for name := range s.CatalogFailures {
		if strings.HasPrefix(name, "mutate/") && strings.HasSuffix(name, ".json") {
			set[name] = true
		}
	}
	var names []string
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
func (s *Snapshots) RecordedFile(ctx context.Context, name string) (string, snapport.Failure) {
	if ctx.Err() != nil {
		return "", &snapport.Cancelled{}
	}
	s.MetadataReads = append(s.MetadataReads, name)
	if failure := s.CatalogFailures[name]; failure != nil {
		return "", failure
	}
	if file, ok := s.CatalogFiles[name]; ok {
		return file, nil
	}
	for key, value := range s.Values {
		if mutationCatalogName(key) == name {
			return value.File, nil
		}
	}
	return "", nil
}

// Only complete model keys acquire a canonical physical name. Existing raw
// catalog entries keep their names verbatim and are never mapped back to keys.
func mutationCatalogName(key string) string {
	return filepath.ToSlash(filepath.Join("mutate", filepath.ToSlash(key)+".json"))
}
