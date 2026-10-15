package main

import (
	"testing"

	"github.com/donvargax/itos-cc/features"
)

func TestMemoryInventory(t *testing.T) { features.InventoryContract(t, newMemory(t)) }
