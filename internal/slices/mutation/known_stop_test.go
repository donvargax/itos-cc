package mutation

import (
	"path/filepath"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestAKnownStopNamesItsFileFromTheWorkingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	e := mutants.StaleException{Exception: mutants.Exception{File: "src/board.go", Function: "board#clear"}, Why: mutants.ExceptionGone}
	stop := Handler{Dir: dir}.knownStop(dir, []mutants.StaleException{e})
	if stop.Rule != "mutation.exception-stale" || stop.Exception.Function != "board#clear" || stop.File != filepath.Join("src", "board.go") {
		t.Errorf("the stop is %+v, want mutation.exception-stale of board#clear naming %s", stop, filepath.Join("src", "board.go"))
	}
	if got := (Handler{Dir: dir}).knownStop(dir, nil); got != nil {
		t.Errorf("no stale entry gives the stop %+v, want none", got)
	}
	abs := filepath.Join(filepath.Dir(dir), "elsewhere", "board.go")
	e.File = filepath.ToSlash(abs)
	if got := (Handler{Dir: dir}).knownStop(dir, []mutants.StaleException{e}).File; got != abs {
		t.Errorf("a file outside the directory is named %q, want %q", got, abs)
	}
}
