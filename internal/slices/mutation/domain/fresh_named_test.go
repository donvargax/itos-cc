package domain_test

import (
	"path/filepath"
	"strings"
	"testing"

	sel "github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The memory plan admits named sites as the physical one does
// (porttest.FreshNamedContract, which freshenv runs against both).
func TestMemoryFreshNamedContract(t *testing.T) {
	porttest.FreshNamedContract(t, memoryFreshFixture)
}

// A committed source the frozen tree cannot read is an error, not a site
// that names nothing: no plan, real or memory, exports a file it cannot
// read, so only a tree that loses it shows this.
func TestANamedSiteInAFileTheTreeCannotReadIsAnError(t *testing.T) {
	root := t.TempDir()
	plan := &mutants.FreshPlan{FrozenRoot: root}
	tree := seltest.TreeOf(root, map[string]string{}, nil)
	sites := []mutants.FreshSite{{Argument: "a.go:2:9", File: "a.go", Line: 2, Column: 9}}
	err := mutants.AdmitNamed(t.Context(), plan, tree, &porttest.Inputs{Dir: root}, nil, []string{filepath.Join(root, "a.go")}, []string{"a.go"}, sel.Exclusion{Root: root}, sites)
	if err == nil || !strings.Contains(err.Error(), "read committed a.go") {
		t.Errorf("AdmitNamed = %v, want the unreadable file", err)
	}
}
