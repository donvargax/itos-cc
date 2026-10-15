package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/features"
)

// Timestamp metadata has no representation in the current memory tree. Keep
// this honest until the actual cleanup adapter/fake binds entry timestamps.
func (m *memory) SetModTime(ctx context.Context, _ string, _ time.Time) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, fmt.Errorf("%w: memory entry timestamps have not moved", features.ErrPhysicalUnsupported)
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
