package snapshots

import (
	"context"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// ListedCoverageName is the cached listed tests' coverage of a project,
// from .metrics: one entry, in the folder of the coverage reports, which
// itos-cc keeps out of git, beside the report folders
// --use-existing-coverage reads.
const ListedCoverageName = "coverage/cache/listed.json"

// ownCoverageFolder is where the cached own tests' coverage of each
// coverage plan is, from .metrics: one entry a plan, by its name.
const ownCoverageFolder = "coverage/cache/own"

// listedCoverageVersion is the version of the entries this itos-cc
// writes, the listed tests' and each plan's own; one of a newer version
// cannot be read.
const listedCoverageVersion = 1

// CoverageCache is the domain's CoverageCache over the snapshot Store,
// which writes an entry whole through a rename.
type CoverageCache struct{ Store snapport.Store }

var _ mutants.CoverageCache = CoverageCache{}

// listedCoverageWire is the entry as it is stored: one JSON object.
type listedCoverageWire struct {
	Version     int                  `json:"version"`
	Inputs      map[string]string    `json:"inputs"`
	Listed      []listedWire         `json:"listed"`
	ListedFiles map[string]string    `json:"listed_files"`
	Measured    []string             `json:"measured"`
	Tests       []listedCoverageTest `json:"tests"`
}

// listedWire is a test as the list command printed it.
type listedWire struct {
	ID          string `json:"id"`
	File        string `json:"file,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// listedCoverageTest is a listed test's coverage, the pieces of each file.
type listedCoverageTest struct {
	ID    string                 `json:"id"`
	Files map[string][]pieceWire `json:"files"`
}

// pieceWire is a piece of a file's coverage: its lines, weight and how much
// of it was covered, its key, whether it ran, whether it is a branch, and
// which sources covered it.
type pieceWire struct {
	Start       int     `json:"start"`
	End         int     `json:"end"`
	Total       float64 `json:"total"`
	Covered     float64 `json:"covered"`
	Key         string  `json:"key,omitempty"`
	Ran         bool    `json:"ran,omitempty"`
	Branch      bool    `json:"branch,omitempty"`
	InProcess   bool    `json:"in_process,omitempty"`
	Integration bool    `json:"integration,omitempty"`
}

// Listed reads the entry: nil when there is none, Unreadable when it does
// not decode or is of another version.
func (c CoverageCache) Listed(ctx context.Context) (*mutants.ListedCoverage, snapport.Failure) {
	var wire listedCoverageWire
	found, failure := c.Store.Read(ctx, ListedCoverageName, &wire)
	if failure != nil || !found {
		return nil, failure
	}
	if wire.Version != listedCoverageVersion {
		return nil, &snapport.Unreadable{Name: ListedCoverageName, Reason: "it is of another version"}
	}
	entry := &mutants.ListedCoverage{Inputs: wire.Inputs, ListedFiles: wire.ListedFiles, Measured: wire.Measured}
	for _, l := range wire.Listed {
		entry.Listed = append(entry.Listed, mutants.CachedListing{ID: l.ID, File: l.File, Fingerprint: l.Fingerprint})
	}
	for _, t := range wire.Tests {
		entry.Tests = append(entry.Tests, mutants.CachedTest{ID: t.ID, Files: fromWire(t.Files)})
	}
	return entry, nil
}

// fromWire is each file's pieces as the domain holds them.
func fromWire(files map[string][]pieceWire) map[string][]mutants.CachedPiece {
	out := map[string][]mutants.CachedPiece{}
	for file, pieces := range files {
		kept := []mutants.CachedPiece{}
		for _, p := range pieces {
			kept = append(kept, mutants.CachedPiece{Start: p.Start, End: p.End, Total: p.Total, Covered: p.Covered, Key: p.Key, Ran: p.Ran,
				Branch: p.Branch, InProcess: p.InProcess, Integration: p.Integration})
		}
		out[file] = kept
	}
	return out
}

// toWire is each file's pieces as they are stored.
func toWire(files map[string][]mutants.CachedPiece) map[string][]pieceWire {
	out := map[string][]pieceWire{}
	for file, pieces := range files {
		stored := []pieceWire{}
		for _, p := range pieces {
			stored = append(stored, pieceWire{Start: p.Start, End: p.End, Total: p.Total, Covered: p.Covered, Key: p.Key, Ran: p.Ran,
				Branch: p.Branch, InProcess: p.InProcess, Integration: p.Integration})
		}
		out[file] = stored
	}
	return out
}

// SaveListed writes entry whole, in place of the one there.
func (c CoverageCache) SaveListed(ctx context.Context, entry mutants.ListedCoverage) snapport.Failure {
	wire := listedCoverageWire{Version: listedCoverageVersion, Inputs: entry.Inputs, Listed: []listedWire{}, ListedFiles: entry.ListedFiles,
		Measured: entry.Measured, Tests: []listedCoverageTest{}}
	for _, l := range entry.Listed {
		wire.Listed = append(wire.Listed, listedWire{ID: l.ID, File: l.File, Fingerprint: l.Fingerprint})
	}
	for _, t := range entry.Tests {
		wire.Tests = append(wire.Tests, listedCoverageTest{ID: t.ID, Files: toWire(t.Files)})
	}
	return c.Store.Write(ctx, ListedCoverageName, wire)
}

// ownCoverageWire is a plan's entry as it is stored: one JSON object.
type ownCoverageWire struct {
	Version  int                    `json:"version"`
	Language string                 `json:"language"`
	Dir      string                 `json:"dir"`
	Tests    []string               `json:"tests,omitempty"`
	Sources  []string               `json:"sources"`
	Inputs   map[string]string      `json:"inputs"`
	Files    map[string][]pieceWire `json:"files"`
	Listed   map[string][]pieceWire `json:"listed,omitempty"`
	Proven   []string               `json:"proven,omitempty"`
	Unloaded []string               `json:"unloaded,omitempty"`
}

// ownName is the entry of the plan named name, from .metrics.
func ownName(name string) string {
	return ownCoverageFolder + "/" + name + ".json"
}

// Own reads the entry of the plan named name: nil when there is none,
// Unreadable when it does not decode or is of another version.
func (c CoverageCache) Own(ctx context.Context, name string) (*mutants.OwnCoverage, snapport.Failure) {
	var wire ownCoverageWire
	found, failure := c.Store.Read(ctx, ownName(name), &wire)
	if failure != nil || !found {
		return nil, failure
	}
	if wire.Version != listedCoverageVersion {
		return nil, &snapport.Unreadable{Name: ownName(name), Reason: "it is of another version"}
	}
	return &mutants.OwnCoverage{Language: wire.Language, Dir: wire.Dir, Tests: wire.Tests, Sources: wire.Sources, Inputs: wire.Inputs,
		Files: fromWire(wire.Files), Listed: fromWire(wire.Listed), Proven: wire.Proven, Unloaded: wire.Unloaded}, nil
}

// SaveOwn writes entry whole as the plan named name's, in place of the one
// there.
func (c CoverageCache) SaveOwn(ctx context.Context, name string, entry mutants.OwnCoverage) snapport.Failure {
	return c.Store.Write(ctx, ownName(name), ownCoverageWire{Version: listedCoverageVersion, Language: entry.Language, Dir: entry.Dir,
		Tests: entry.Tests, Sources: entry.Sources, Inputs: entry.Inputs, Files: toWire(entry.Files), Listed: toWire(entry.Listed), Proven: entry.Proven,
		Unloaded: entry.Unloaded})
}
