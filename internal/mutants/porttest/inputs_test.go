package porttest

import (
	"maps"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func memoryInputFixture(t *testing.T, files map[string]string, imports map[string][]string, env map[string]string) InputBindings {
	t.Helper()
	root, err := filepath.Abs(filepath.FromSlash("/T89-memory-project"))
	if err != nil {
		t.Fatal(err)
	}
	input := &Inputs{Dir: root, Files: map[string]string{}, Runtime: mutants.RuntimeInputs{GoVersion: runtime.Version(), Platform: runtime.GOOS, Architecture: runtime.GOARCH, Env: maps.Clone(env)}}
	for name, text := range files {
		input.Files[filepath.Join(root, filepath.FromSlash(name))] = text
	}
	if imports != nil {
		input.Imports = map[string][]string{}
		for name, edges := range imports {
			path := filepath.Join(root, filepath.FromSlash(name))
			input.Imports[path] = []string{}
			for _, edge := range edges {
				input.Imports[path] = append(input.Imports[path], filepath.Join(root, filepath.FromSlash(edge)))
			}
		}
	}
	return InputBindings{Toolchain: input, Fingerprints: input, Reach: input, Root: root}
}
func TestMemoryFingerprintsContract(t *testing.T) { FingerprintsContract(t, memoryInputFixture) }
func TestMemoryToolchainContract(t *testing.T)    { ToolchainContract(t, memoryInputFixture) }
func TestMemoryReachContract(t *testing.T)        { ReachContract(t, memoryInputFixture) }

func TestGoCoverageNeedsARegularModuleMarker(t *testing.T) {
	b := memoryInputFixture(t, map[string]string{"a.go": "package board\n"}, nil, nil)
	input := b.Fingerprints.(*Inputs)
	input.Dirs = map[string]bool{filepath.Join(b.Root, "go.mod"): true}
	if _, failure := input.Coverage(t.Context(), filepath.Join(b.Root, "a.go"), b.Root, "producer", nil); failure == nil {
		t.Fatal("a directory named go.mod admitted coverage provenance")
	}
}
