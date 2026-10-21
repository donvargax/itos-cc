package worktrees

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestWorkersContract(t *testing.T) {
	porttest.WorkersContract(t, func(t *testing.T, baseline string) (mutants.Workers, mutants.Command) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("original\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("T88_CONTRACT_CHILD", "yes")
		t.Setenv("T88_CONTRACT_BASELINE", baseline)
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		return Workers{Platform: runtime.GOOS}, mutants.Command{Root: root, Dir: root, Args: []string{exe, "-test.run=^TestWorkersContractHelper$"}}
	})
}
func TestWorkersContractHelper(t *testing.T) {
	if os.Getenv("T88_CONTRACT_CHILD") == "" {
		return
	}
	text, err := os.ReadFile("source.txt")
	if err != nil {
		t.Fatal(err)
	}
	if baseline := os.Getenv("T88_CONTRACT_BASELINE"); baseline != "" {
		if _, err := fmt.Fprint(os.Stdout, baseline); err != nil {
			t.Fatal(err)
		}
		os.Exit(1)
	}
	if string(text) != "original\n" {
		os.Exit(1)
	}
}
