package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// A kill by listed tests (ScopeListed) rests on files no import names: the
// file that defines each of its tests, as the list command named it
// (Snapshot.Listed), and the support files every listed test depends on
// (mutation.tests.support). A snapshot that records such an outcome records
// the hashes of both, and the outcome holds while they are unchanged. No
// list command runs to judge it: deleting a test changes its file.

// SupportHashes is the SHA-256 of each file under root that globs, from
// root, match, by its slash-separated path from root; nil without globs.
func SupportHashes(root string, globs []string) (map[string]string, error) {
	if len(globs) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, glob := range globs {
		paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(glob)))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil, err
			}
			if out[filepath.ToSlash(rel)] == "" {
				out[filepath.ToSlash(rel)] = fileHash(path)
			}
		}
	}
	return out, nil
}

// fileHash is the SHA-256 of the file at path, or "" when it cannot be
// read.
func fileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

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
