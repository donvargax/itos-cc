package mutate

import (
	"os"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// annotate replaces the summary comment at the end of path with one for
// snap, each survivor excepted by its reason in excepted, writing the file
// only when the summary changed. The comment sits after every function, so
// it never changes a function's hash.
func annotate(path string, spec *lang.Spec, snap Snapshot, excepted map[string]string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := mutants.Annotated(string(src), spec.Comment, Annotation(snap, spec.Comment, excepted))
	if updated == string(src) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), info.Mode().Perm())
}
