package graph

import (
	"context"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Mutation is the port through which graph learns what mutation check
// knows of each function (q-95, D1): graph owns it and imports nothing of
// the mutation slice, and cmd binds it to the judgment that slice exports,
// so check's rule stays the mutation slice's and the two cannot drift
// (GRAPH-38). What it hands graph to keep, a parsed file's facts and a
// file's snapshot, graph keeps in its caches and hands back, never reading
// it.
type Mutation interface {
	// Facts is what the judgment needs later of the units of f, a source
	// parsed: graph keeps it with the file in its cache, so an unchanged
	// file is never parsed again for it.
	Facts(f *lang.File) any
	// SnapshotPath is the file the snapshot of the source whose key is
	// key, its path from the project root top, is read from, which graph
	// watches.
	SnapshotPath(top, key string) string
	// Snapshot is that snapshot, read where mutation check reads it; nil
	// for none, and one that cannot be read is none too, so the graph still
	// builds. Its error is a missing binding.
	Snapshot(ctx context.Context, top, key string) (any, error)
	// Support is the hashes of the support files of the tests the
	// project whose root is top lists, by path, which a listed outcome
	// rests on; nil when it lists none or they cannot be read.
	Support(ctx context.Context, top string) map[string]string
	// Tests is what a snapshot records of the test files tests of the
	// project whose root is top, as mutation check hashes them.
	Tests(ctx context.Context, top string, tests []string) (map[string]string, error)
	// Judge is what mutation check calls each unit of a source file, in
	// the order of the units its facts were made of.
	Judge(ctx context.Context, file Judged) []UnitMutation
}

// Judged is a source file as Mutation.Judge judges it: its project root,
// its absolute path, the snapshot Mutation.Snapshot read of it and the facts
// Mutation.Facts made of it, both as they were handed over, the hashes of
// the test files that import it now (Mutation.Tests), and Support, its
// project's.
type Judged struct {
	Top, Path string
	Snapshot  any
	Facts     any
	Tests     map[string]string
	Support   map[string]string
}

// UnitMutation is what mutation check knows of one function: whether a
// snapshot records results for it, how many of its mutants were killed,
// survived or went uncovered, and whether those results are stale, as
// mutation check calls them.
type UnitMutation struct {
	Mutated                     bool
	Killed, Survived, Uncovered int
	Stale                       bool
}
