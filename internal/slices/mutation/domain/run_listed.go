package domain

import (
	"context"
	"path/filepath"
)

// markListedStale marks Stale each entry of the snapshot a run writes that
// it did not judge and whose listed outcomes rest on files that changed:
// the snapshot records those files as they are now, so the entry must not
// read as fresh until a run judges it again.
func (s *Execution) markListedStale() {
	if s.judged == nil || s.listedChange == nil {
		return
	}
	judged := map[string]bool{}
	for _, id := range s.result.Judged {
		judged[id] = true
	}
	units := s.result.Snapshot.Units
	for i := range units {
		if !judged[UnitID(units[i].Namespace, units[i].Name)] && s.listedChange.Changed(units[i].Mutants) {
			units[i].Stale = true
		}
	}
}

// listedHash keeps unreadable observations best effort without turning
// optional freshness observations into newly fatal errors.
func listedHash(ctx context.Context, root, file string, fp Fingerprints) string {
	hashes, failure := fp.Tests(ctx, root, []string{filepath.Join(root, filepath.FromSlash(file))})
	if failure != nil {
		return ""
	}
	return hashes[file]
}

// ListedChangedWithInputs is ListedChangedListing without the list: a test
// recorded with a fingerprint whose file changed counts as changed.
func ListedChangedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp Fingerprints) *ListedChange {
	c, _ := ListedChangedListing(ctx, s, root, support, fp, nil, nil)
	return c
}

// ListedChangedListing is how the tests and files the listed outcomes of s
// rest on changed since it recorded them, the files hashed through fp under
// root, support the support files' hashes now. A test recorded with a
// fingerprint is judged by listing, run, and reported to progress, only when
// its file changed, at most once (ListOnce); a list that fails is its
// error. Without listing such a test counts as changed.
func ListedChangedListing(ctx context.Context, s *Snapshot, root string, support map[string]string, fp Fingerprints, listing *ListOnce, progress Progress) (*ListedChange, error) {
	if s == nil || !HasListed(s.Units) {
		return nil, nil
	}
	now := map[string]string{}
	for _, file := range s.Listed {
		if file != "" {
			now[file] = listedHash(ctx, root, file, fp)
		}
	}
	var current *ListedInventory
	if listing != nil && s.needsListing(now) {
		inv, err := listing.List(ctx, progress)
		if err != nil {
			return nil, err
		}
		current = &inv
	}
	return s.listedChanged(now, support, current), nil
}

// ListedFingerprintsOf is the fingerprint of each test listed names, by its
// ID, that has one: as inv, the listing, prints it when the list ran
// (ran) and lists the test, else as previous recorded it. nil when none
// has one.
func ListedFingerprintsOf(listed map[string]string, inv ListedInventory, ran bool, previous *Snapshot) map[string]string {
	now := map[string]bool{}
	for _, test := range inv.Tests {
		now[test.ID] = true
	}
	var out map[string]string
	for id := range listed {
		var fingerprint string
		switch {
		case ran && now[id]:
			fingerprint = inv.fingerprint(id)
		case previous != nil:
			fingerprint = previous.ListedFingerprints[id]
		}
		if fingerprint == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[id] = fingerprint
	}
	return out
}

// files is the file of each test inv lists, by its ID.
func (inv ListedInventory) files() map[string]string {
	out := map[string]string{}
	for _, test := range inv.Tests {
		out[test.ID] = test.File
	}
	return out
}

// recordListedWithInputs records in s, once its Listed is set, the hash of
// each file a listed outcome rests on, the support files' and fingerprints,
// that of each test listed with one.
func recordListedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp Fingerprints, fingerprints map[string]string) {
	// Ordinary observations use the caller context; a failed optional read keeps
	// its empty hash. A cancelled caller cannot publish the resulting snapshot.
	s.ListedFiles, s.Support, s.ListedFingerprints = nil, nil, nil
	if !HasListed(s.Units) {
		return
	}
	s.ListedFingerprints = fingerprints
	for _, file := range s.Listed {
		if file == "" {
			continue
		}
		if s.ListedFiles == nil {
			s.ListedFiles = map[string]string{}
		}
		s.ListedFiles[file] = listedHash(ctx, root, file, fp)
	}
	if len(support) > 0 {
		s.Support = map[string]string{}
		for path, hash := range support {
			s.Support[path] = hash
		}
	}
}
