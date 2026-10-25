package worktrees

import (
	"fmt"
	"os"
	"os/exec"
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
		t.Setenv(contractChild, "yes")
		t.Setenv("T88_CONTRACT_BASELINE", baseline)
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		return Workers{Platform: runtime.GOOS, Prepare: ownCoverDir}, mutants.Command{Root: root, Dir: root, Args: []string{exe, "-test.run=^TestWorkersContractHelper$"}}
	})
}

// ownCoverDir gives cmd, this test binary run again as the contract's
// child, the GOCOVERDIR go test -cover gave this one, which the worker
// keeps out of what it starts: without it a child built with -cover warns
// on its output, which the contract reads whole.
func ownCoverDir(cmd *exec.Cmd) {
	if dir, ok := os.LookupEnv("GOCOVERDIR"); ok {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+dir)
	}
}

func TestWorkersContractHelper(t *testing.T) {
	if os.Getenv(contractChild) == "" {
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
