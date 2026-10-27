package domain

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/snapshot"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
)

// CoverageCache keeps, between runs, the listed tests' coverage a complete
// run measured (slice-69, issue #67): .metrics/coverage/cache, one entry
// for the project, which a run reuses while nothing it was measured from
// changed. Deleting it is always safe.
type CoverageCache interface {
	// Listed is the entry kept, nil when there is none. One that cannot be
	// read, or was written by a newer itos-cc, is Unreadable.
	Listed(ctx context.Context) (*ListedCoverage, snapport.Failure)
	// SaveListed keeps entry, whole, in place of the one kept, so a run
	// reading it at the same time never reads half of one.
	SaveListed(ctx context.Context, entry ListedCoverage) snapport.Failure
}

// CachedPiece is a piece of a listed test's coverage of a file, as the
// cache keeps it: coverage.Piece.
type CachedPiece = coverage.Piece

// ListedCoverage is the listed tests' coverage a run measured, and what it
// was measured from: the inputs strict coverage evidence records of each
// measured file's build root (Fingerprints.Coverage), what the list command
// printed, and each listed test's file. Paths are slash-separated, from the
// project root.
type ListedCoverage struct {
	// Inputs are the hashes Fingerprints.Coverage gives of each measured
	// file's build root, by its name there.
	Inputs map[string]string
	// Listed is each test the list command printed, in its order.
	Listed []CachedListing
	// ListedFiles is the hash of each file a listed test is defined in,
	// "" for one that could not be read.
	ListedFiles map[string]string
	// Measured is each file the coverage was measured of, sorted.
	Measured []string
	// Tests is the coverage of each listed test that wrote any, in the
	// list's order.
	Tests []CachedTest
}

// CachedListing is a test as the list command printed it: its ID, its
// file, "" for none, and its fingerprint, "" for none.
type CachedListing struct {
	ID, File, Fingerprint string
}

// CachedTest is a listed test's coverage: the pieces of each measured file
// it covered, by the file's path.
type CachedTest struct {
	ID    string
	Files map[string][]CachedPiece
}

// ListedReuse is a run's use of the cached listed tests' coverage: the
// cache, whether the run keeps what it measures (a complete run's built-in
// measurement), and where it says what it did.
type ListedReuse struct {
	Cache    CoverageCache
	Write    bool
	Progress Progress
	// kept is the entry Serve read when its inputs were the run's: the one
	// Keep adds the run's files to.
	kept *ListedCoverage
}

// ListedKey is what a run's listed tests' coverage rests on, taken before
// any coverage command runs: the entry it would keep, holding no coverage,
// and the project root its paths are from.
type ListedKey struct {
	root  string
	entry ListedCoverage
}

// The events of a run's use of the cache: its coverage reused, or the
// reason, Failure, an entry does not serve it.
const (
	ListedCoverageReused   = "listed-coverage-reused"
	ListedCoverageUnserved = "listed-coverage-unserved"
)

// ListedUnserved is why the cached listed tests' coverage does not serve a
// run: the inputs that changed, by their names, the files it measured none
// of, or that it cannot be read.
type ListedUnserved struct {
	Changed, Unmeasured []string
	Unreadable          bool
}

func (u ListedUnserved) Error() string {
	switch {
	case u.Unreadable:
		return "it cannot be read"
	case len(u.Changed) > 0:
		var named []string
		for _, name := range u.Changed {
			named = append(named, name+" changed")
		}
		return firstThree(named)
	}
	return "it measured no " + firstThree(u.Unmeasured)
}

// firstThree is up to three of names, by commas, then how many more.
func firstThree(names []string) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:3], ", ") + " and " + strconv.Itoa(len(names)-3) + " more"
}

// KeyListed is the key of the listed tests' coverage of sources, measured
// at root after the list command printed inv: nil when the cache cannot
// serve the run, as a source's scope is one strict coverage refuses
// (mutation.coverage-unsupported) or an input cannot be read.
func KeyListed(ctx context.Context, tools Toolchain, fp Fingerprints, root string, sources []string, inv ListedInventory, support map[string]string) *ListedKey {
	if fp == nil || len(sources) == 0 {
		return nil
	}
	key := &ListedKey{root: root, entry: ListedCoverage{Inputs: map[string]string{}, ListedFiles: map[string]string{}}}
	measured := map[string]string{}
	for _, source := range slices.Compact(slices.Sorted(slices.Values(sources))) {
		spec := lang.Detect(source)
		if spec == nil {
			return nil
		}
		rel := snapshot.Key(root, source)
		key.entry.Measured = append(key.entry.Measured, rel)
		if measured[rel] == spec.Name {
			// Its build root's inputs are in already.
			continue
		}
		if unsupported, failure := fp.Unsupported(ctx, spec.Name, source, root); failure != nil || unsupported != "" {
			return nil
		}
		_, inputs, err := CoverageInputsFor(ctx, tools, fp, source, root, false, false, support)
		if err != nil {
			return nil
		}
		for name, hash := range inputs {
			key.entry.Inputs[name] = hash
			measured[name] = spec.Name
		}
	}
	for _, test := range inv.Tests {
		key.entry.Listed = append(key.entry.Listed, CachedListing{ID: test.ID, File: test.File, Fingerprint: inv.fingerprint(test.ID)})
		if test.File != "" {
			key.entry.ListedFiles[test.File] = listedHash(ctx, root, test.File, fp)
		}
	}
	return key
}

// Serve is the cached coverage of each listed test, its paths at the key's
// root, when the kept entry was measured from what key says and of every
// file it measures; nil otherwise, having said why an entry there does not
// serve the run. A nil reuse or key serves nothing.
func (u *ListedReuse) Serve(ctx context.Context, key *ListedKey) *coverage.TestCoverage {
	if u == nil || key == nil || u.Cache == nil {
		return nil
	}
	entry, failure := u.Cache.Listed(ctx)
	if _, cancelled := failure.(*snapport.Cancelled); cancelled {
		return nil
	}
	if failure != nil {
		report(u.Progress, RunEvent{Kind: ListedCoverageUnserved, Failure: ListedUnserved{Unreadable: true}})
		return nil
	}
	if entry == nil {
		return nil
	}
	if changed := key.entry.changed(*entry); len(changed) > 0 {
		report(u.Progress, RunEvent{Kind: ListedCoverageUnserved, Failure: ListedUnserved{Changed: changed}})
		return nil
	}
	u.kept = entry
	var lacking []string
	for _, file := range key.entry.Measured {
		if !slices.Contains(entry.Measured, file) {
			lacking = append(lacking, file)
		}
	}
	if len(lacking) > 0 {
		report(u.Progress, RunEvent{Kind: ListedCoverageUnserved, Failure: ListedUnserved{Unmeasured: lacking}})
		return nil
	}
	report(u.Progress, RunEvent{Kind: ListedCoverageReused})
	tc := &coverage.TestCoverage{}
	for _, test := range entry.Tests {
		files := map[string][]coverage.Piece{}
		for file, pieces := range test.Files {
			files[key.path(file)] = pieces
		}
		tc.Add(test.ID, coverage.FromPieces(files))
	}
	return tc
}

// Keep keeps what report measured of the listed tests under key, when the
// run writes and its built-in measurement succeeded whole: the listed
// tests' coverage run passed, and every plan measured its language, with
// nothing missing, failed or left unmeasured by its collector. Its files
// are added to the entry Serve read when that was measured from the same
// inputs; otherwise the entry is replaced. A cache it cannot write is
// left as it is.
func (u *ListedReuse) Keep(ctx context.Context, key *ListedKey, report *coverage.Report) {
	tc := report.Tests()
	if u == nil || !u.Write || key == nil || u.Cache == nil || tc == nil ||
		len(report.Missing()) > 0 || len(report.Failed()) > 0 || len(report.IntegrationMissing()) > 0 {
		return
	}
	entry := key.entry
	entry.Measured = slices.Clone(entry.Measured)
	byID := map[string]map[string][]CachedPiece{}
	if u.kept != nil {
		entry.Measured = append(entry.Measured, u.kept.Measured...)
		for _, test := range u.kept.Tests {
			byID[test.ID] = maps.Clone(test.Files)
		}
	}
	slices.Sort(entry.Measured)
	entry.Measured = slices.Compact(entry.Measured)
	for _, id := range tc.IDs() {
		measured := tc.Of(id)
		if byID[id] == nil {
			byID[id] = map[string][]CachedPiece{}
		}
		for _, file := range measured.Files() {
			byID[id][snapshot.Key(key.root, file)] = measured.Pieces(file)
		}
	}
	for _, listed := range entry.Listed {
		if files, ok := byID[listed.ID]; ok {
			entry.Tests = append(entry.Tests, CachedTest{ID: listed.ID, Files: files})
		}
	}
	_ = u.Cache.SaveListed(ctx, entry)
}

// path is file, a path the cache keeps, at the key's root.
func (k *ListedKey) path(file string) string {
	if path := filepath.FromSlash(file); filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(k.root, filepath.FromSlash(file))
}

// changed is the name of each input of k that kept does not have as k has
// it, sorted: an input file by its path, a support file by its path, a
// listed test by its ID and a listed test's file by its path.
func (k ListedCoverage) changed(kept ListedCoverage) []string {
	names := map[string]bool{}
	for _, name := range differing(k.Inputs, kept.Inputs) {
		names[strings.TrimPrefix(name, "support:")] = true
	}
	for _, name := range differing(k.ListedFiles, kept.ListedFiles) {
		names[name] = true
	}
	listing := func(entries []CachedListing) map[string]string {
		out := map[string]string{}
		for _, e := range entries {
			out[e.ID] = fmt.Sprintf("%q %q", e.File, e.Fingerprint)
		}
		return out
	}
	for _, id := range differing(listing(k.Listed), listing(kept.Listed)) {
		names[id] = true
	}
	return slices.Sorted(maps.Keys(names))
}

// differing is each key a and b do not hold alike.
func differing(a, b map[string]string) []string {
	var out []string
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			out = append(out, key)
		}
	}
	for key := range b {
		if _, ok := a[key]; !ok {
			out = append(out, key)
		}
	}
	return out
}

// MeasureReusing is m's measurement of req, the listed tests' coverage
// beside it (perTest) served from the cache when reuse serves key, and
// kept there when reuse writes and the measurement succeeded.
func MeasureReusing(ctx context.Context, m coverage.MutationMeasurer, req covport.Request, perTest *coverage.PerTest, reuse *ListedReuse, key *ListedKey) (*coverage.Report, covport.Failure) {
	if perTest == nil {
		return m.MeasureListed(ctx, req, nil)
	}
	if tc := reuse.Serve(ctx, key); tc != nil {
		report, failure := m.MeasureListed(ctx, req, nil)
		if failure == nil {
			report.SetTests(tc)
		}
		return report, failure
	}
	report, failure := m.MeasureListed(ctx, req, perTest)
	if failure == nil {
		reuse.Keep(ctx, key, report)
	}
	return report, failure
}
