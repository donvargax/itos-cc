package mutate

import (
	"context"
	"path/filepath"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/testinputs"
)

func fileHash(path string) string { return testinputs.FileHash(path) }

// recordListed records in s what its listed outcomes rest on: the hash of
// each file Listed names, now, under root, and support, the support files'
// hashes now. A snapshot with no listed outcome records neither.
func recordListed(s *Snapshot, root string, support map[string]string) {
	s.ListedFiles, s.Support = nil, nil
	if !hasListed(s.Units) {
		return
	}
	for _, file := range s.Listed {
		if file == "" {
			continue
		}
		if s.ListedFiles == nil {
			s.ListedFiles = map[string]string{}
		}
		s.ListedFiles[file] = fileHash(filepath.Join(root, filepath.FromSlash(file)))
	}
	if len(support) > 0 {
		s.Support = map[string]string{}
		for path, hash := range support {
			s.Support[path] = hash
		}
	}
}

// ListedChanged is how the files the listed outcomes of s rest on, under
// root, differ now from those it recorded, support being the support
// files' hashes now (SupportHashes); nil while they are the same or s
// records no listed outcome.
func ListedChanged(s *Snapshot, root string, support map[string]string) *ListedChange {
	if s == nil || !hasListed(s.Units) {
		return nil
	}
	now := map[string]string{}
	for _, file := range s.Listed {
		if file != "" {
			now[file] = fileHash(filepath.Join(root, filepath.FromSlash(file)))
		}
	}
	return s.ListedChanged(now, support)
}

// markListedStale marks Stale each entry of the snapshot a run writes that
// it did not judge and whose listed outcomes rest on files that changed:
// the snapshot records those files as they are now, so the entry must not
// read as fresh until a run judges it again.
func (s *fileState) markListedStale() {
	if s.judged == nil || s.listedChange == nil {
		return
	}
	judged := map[string]bool{}
	for _, id := range s.result.Judged {
		judged[id] = true
	}
	units := s.result.Snapshot.Units
	for i := range units {
		if !judged[unitID(units[i].Namespace, units[i].Name)] && len(s.listedChange.Files(units[i].Mutants)) > 0 {
			units[i].Stale = true
		}
	}
}

// listedHash preserves the old best-effort unreadable-file result without
// turning optional cleanup or freshness observations into newly fatal errors.
func listedHash(ctx context.Context, root, file string, fp mutants.Fingerprints) string {
	if fp == nil {
		return fileHash(filepath.Join(root, filepath.FromSlash(file)))
	}
	hashes, failure := fp.Tests(ctx, root, []string{filepath.Join(root, filepath.FromSlash(file))})
	if failure != nil {
		return ""
	}
	return hashes[file]
}
func listedChangedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp mutants.Fingerprints) *ListedChange {
	if fp == nil {
		return ListedChanged(s, root, support)
	}
	if s == nil || !hasListed(s.Units) {
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
func recordListedWithInputs(ctx context.Context, s *Snapshot, root string, support map[string]string, fp mutants.Fingerprints) {
	// Legacy finalization read these files even on an externally stopped run.
	// Keep only those retained best-effort hashes uncancelled; publication and
	// ordinary current observations still receive their original caller ctx.
	ctx = context.WithoutCancel(ctx)
	if fp == nil {
		recordListed(s, root, support)
		return
	}
	s.ListedFiles, s.Support = nil, nil
	if !hasListed(s.Units) {
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
