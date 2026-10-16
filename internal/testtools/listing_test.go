package testtools_test

import (
	"bytes"
	"os"
	"runtime"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	"github.com/donvargax/itos-cc/internal/testtools"
)

// Actual native helper streams keep list rows independent of diagnostics/status.
func TestRealListingContract(t *testing.T) {
	porttest.ListingContract(t, func(t *testing.T, fails bool) (mutants.TestLister, mutants.Command, *bytes.Buffer) {
		helper, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		flag := "--T92-list"
		if fails {
			flag = "--T92-list-fails"
		}
		root := t.TempDir()
		command := mutants.Command{Root: root, Dir: root, Shell: "\"" + helper + "\" " + flag}
		log := &bytes.Buffer{}
		return testtools.Lister{Platform: runtime.GOOS, Log: log}, command, log
	})
}
