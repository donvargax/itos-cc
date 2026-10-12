package project

import (
	"context"

	"github.com/donvargax/itos-cc/lang"
)

// ParseFile reads and parses path, refusing a language lang does not know
// before it reads anything.
func ParseFile(ctx context.Context, path string) (*lang.File, error) {
	return lang.ParseFile(path)
}

// FindUp returns the nearest directory at or above path's directory that
// contains one of markers, or "" when none does.
func FindUp(ctx context.Context, path string, markers ...string) string {
	return lang.FindUp(path, markers...)
}
