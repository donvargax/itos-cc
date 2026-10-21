package testinputs

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
				out[filepath.ToSlash(rel)] = FileHash(path)
			}
		}
	}
	return out, nil
}

// fileHash is the SHA-256 of the file at path, or "" when it cannot be
// read.
func FileHash(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // hash caller-selected support files, preserving existing glob and outside-relative behavior
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
