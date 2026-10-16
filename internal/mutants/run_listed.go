package mutants

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
		if !judged[UnitID(units[i].Namespace, units[i].Name)] && len(s.listedChange.Files(units[i].Mutants)) > 0 {
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
func listedChangedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp Fingerprints) *ListedChange {
	if s == nil || !HasListed(s.Units) {
		return nil
	}
	now := map[string]string{}
	for _, file := range s.Listed {
		if file != "" {
			now[file] = listedHash(ctx, root, file, fp)
		}
	}
	return s.ListedChanged(now, support)
}
func recordListedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp Fingerprints) {
	// Ordinary observations use the caller context; a failed optional read keeps
	// its empty hash. A cancelled caller cannot publish the resulting snapshot.
	s.ListedFiles, s.Support = nil, nil
	if !HasListed(s.Units) {
		return
	}
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
