package workdir

import (
	"path/filepath"
	"testing"

	langport "github.com/donvargax/itos-cc/internal/lang/port"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// Surroundings keep the contract every Surroundings keeps, the fake's
// included, answering from the disk of the working directory.
func TestSurroundingsKeepTheContract(t *testing.T) {
	langporttest.SurroundingsContract(t, func(t *testing.T, files map[string]string) (langport.Surroundings, string) {
		return Surroundings{}, workingDir(t, files)
	})
}

// A done context reads nothing.
func TestSurroundingsReadNothingOnceTheContextIsDone(t *testing.T) {
	tree(t)
	if text, ok := (Surroundings{}).Read(cancelled(t), filepath.Join("pkg", "sum.go")); ok || text != nil {
		t.Errorf("Read() with a done context = %q, %t; want nothing", text, ok)
	}
}
