package domain_test

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The memory plan admits named sites as the physical one does
// (porttest.FreshNamedContract, which freshenv runs against both).
func TestMemoryFreshNamedContract(t *testing.T) {
	porttest.FreshNamedContract(t, memoryFreshFixture)
}
