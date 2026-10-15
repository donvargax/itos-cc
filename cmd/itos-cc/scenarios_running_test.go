package main

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

func (m *memory) Platform() string { return runtime.GOOS }
func (m *memory) Start(ctx context.Context, _ features.Run) (features.Running, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, features.ErrPhysicalUnsupported
}
func (m *memory) FixturePeers(ctx context.Context) (features.FixturePeers, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, features.ErrPhysicalUnsupported
}

func TestMemoryPhysicalExecutionIsExplicitlyUnsupported(t *testing.T) {
	m := &memory{}
	if m.Platform() != runtime.GOOS {
		t.Fatal("driver platform is not actual host value")
	}
	if _, err := m.Start(t.Context(), features.Run{}); !errors.Is(err, features.ErrPhysicalUnsupported) {
		t.Fatalf("memory Start: %v", err)
	}
	if _, err := m.FixturePeers(t.Context()); !errors.Is(err, features.ErrPhysicalUnsupported) {
		t.Fatalf("memory FixturePeers: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := m.Start(ctx, features.Run{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled memory Start: %v", err)
	}
	if _, err := m.FixturePeers(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled memory peers: %v", err)
	}
}
