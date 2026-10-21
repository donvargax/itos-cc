package testinputs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// TestHashes names actual test bytes from the project root; an outside path
// remains whole, exactly as the stored test freshness contract requires.
func TestHashes(root string, tests []string) (map[string]string, error) {
	out := map[string]string{}
	for _, path := range tests {
		data, err := os.ReadFile(path) //nolint:gosec // fingerprint the actual selected test file
		if err != nil {
			return nil, err
		}
		name := path
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			name = rel
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(name)] = hex.EncodeToString(sum[:])
	}
	return out, nil
}
