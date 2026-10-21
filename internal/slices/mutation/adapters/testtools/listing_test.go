package testtools_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
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
		// The configured shell still executes a genuine native helper. An owned
		// relative filename avoids injecting quoted executable paths into cmd /C.
		data, err := os.ReadFile(helper)
		if err != nil {
			t.Fatal(err)
		}
		const name = "listing-helper.exe"
		if err := os.WriteFile(filepath.Join(root, name), data, 0700); err != nil {
			t.Fatal(err)
		}
		command := mutants.Command{Root: root, Dir: root, Shell: "." + string(filepath.Separator) + name + " " + flag}
		log := &bytes.Buffer{}
		return testtools.Lister{Platform: runtime.GOOS, Log: log}, command, log
	})
}
