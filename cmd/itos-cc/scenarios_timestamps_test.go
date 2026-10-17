package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/features"
)

// The memory tree records no modification time of an entry, so it cannot
// age one: ID-SNAP-07 skips here with this reason, never passing on the
// requested time, until the tree records and reports real timestamps and a
// fake of the coverage runner's cleanup reads them.
func (m *memory) SetModTime(ctx context.Context, _ string, _ time.Time) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, fmt.Errorf("%w: the memory tree records no modification times", features.ErrPhysicalUnsupported)
}

func TestMemoryTimestampRefusalAndCancellation(t *testing.T) {
	m := newMemory(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := m.SetModTime(ctx, "/missing", time.Time{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled timestamp: %v", err)
	}
	if _, err := m.SetModTime(t.Context(), "/missing", time.Time{}); !errors.Is(err, features.ErrPhysicalUnsupported) {
		t.Fatalf("unmoved timestamp: %v", err)
	}
}
