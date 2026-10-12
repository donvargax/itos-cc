package project

import (
	"context"
	"fmt"
	"os"

	"github.com/donvargax/itos-cc/lang"
)

// ParseFile reads and parses path, lang naming it through Surroundings, the
// disk. A language lang does not know is refused before anything is read.
func ParseFile(ctx context.Context, path string) (*lang.File, error) {
	spec := lang.Detect(path)
	if spec == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return lang.Parse(ctx, Surroundings{}, spec, path, src)
}

// FindUp returns the nearest directory at or above path's directory that
// contains one of markers on disk, or "" when none does, by lang's walk up
// through Surroundings.
func FindUp(ctx context.Context, path string, markers ...string) string {
	return lang.Nearest(ctx, Surroundings{}, path, markers...)
}
