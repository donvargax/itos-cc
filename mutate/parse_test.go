package mutate

import (
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/project"
	"testing"
)

func parse(t *testing.T, path, src string) *lang.File {
	t.Helper()
	f, err := lang.Parse(t.Context(), project.Surroundings{}, lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}
