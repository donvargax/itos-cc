package domain

import "maps"

// GoCoverageChanges names every missing, added, or changed fingerprint input.
func GoCoverageChanges(recorded, current map[string]string) []string {
	if maps.Equal(recorded, current) {
		return nil
	}
	var changed []string
	for path, hash := range recorded {
		if now, ok := current[path]; !ok || now != hash {
			changed = append(changed, path)
		}
	}
	for path, hash := range current {
		if before, ok := recorded[path]; !ok || before != hash {
			changed = append(changed, path)
		}
	}
	return UniqueSorted(changed)
}
