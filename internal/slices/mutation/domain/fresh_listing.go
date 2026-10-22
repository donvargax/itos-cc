package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// ParseFreshTests preserves counted preparation's strict unique-ID/file syntax,
// separately from ordinary listing's first-ID-wins admission. A third column,
// a test's fingerprint, is no part of its file; a counted run judges every
// mutant afresh, so it needs none.
func ParseFreshTests(output string) ([]coverage.Test, error) {
	var tests []coverage.Test
	seen := map[string]bool{}
	for _, raw := range strings.Split(output, "\n") {
		id, rest, _ := strings.Cut(strings.TrimRight(raw, "\r"), "\t")
		file, _, _ := strings.Cut(rest, "\t")
		id, file = strings.TrimSpace(id), filepath.ToSlash(strings.TrimSpace(file))
		if id == "" {
			continue
		}
		if strings.ContainsAny(id, "\r\n\t") || seen[id] {
			return nil, fmt.Errorf("malformed or duplicate listed test ID %q", id)
		}
		if file != "" {
			if filepath.IsAbs(file) || file == ".git" || strings.HasPrefix(file, ".git/") || strings.HasPrefix(file, "../") || strings.Contains(file, "\\") {
				return nil, fmt.Errorf("listed test %q names unsupported file %q", id, file)
			}
			clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))
			if clean == ".." || strings.HasPrefix(clean, "../") || clean != file {
				return nil, fmt.Errorf("listed test %q names non-canonical frozen file %q", id, file)
			}

		}
		seen[id] = true
		tests = append(tests, coverage.Test{ID: id, File: file})
	}
	if len(tests) == 0 {
		return nil, errors.New("frozen mutation.tests.list produced no test IDs")
	}
	return tests, nil
}
