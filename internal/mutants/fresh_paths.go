package mutants

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func CanonicalFreshPaths(paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	out := make(map[string]bool, len(paths))
	for _, raw := range paths {
		if filepath.IsAbs(raw) {
			return nil, FreshRefused{Kind: FreshPath, Cause: fmt.Errorf("bad fresh plan path: must be root-relative: %q", raw)}
		}
		rel := filepath.ToSlash(filepath.Clean(raw))
		if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "\\") {
			return nil, FreshRefused{Kind: FreshPath, Cause: fmt.Errorf("bad fresh plan path: escapes or ambiguously names the repository: %q", raw)}
		}
		out[path.Clean(rel)] = true
	}
	return out, nil
}

func SelectsFreshPath(rel string, selected map[string]bool) bool {
	if len(selected) == 0 {
		return true
	}
	for chosen := range selected {
		if chosen == "." || rel == chosen || strings.HasPrefix(rel, strings.TrimSuffix(chosen, "/")+"/") {
			return true
		}
	}
	return false
}
