package main

import (
	"testing"

	"github.com/donvargax/itos-cc/features"
)

func TestMemoryExecutableBytes(t *testing.T) { features.ExecutableContract(t, newMemory(t)) }
