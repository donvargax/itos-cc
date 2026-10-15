package mutate

import (
	"fmt"
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

func describe(sites []Site) []string {
	var out []string
	for _, s := range sites {
		out = append(out, fmt.Sprintf("%d:%s>%s", s.Line, s.Original, s.Replacement))
	}
	return out
}

func byUnit(f *lang.File, outcome map[string]string) []string {
	var out []string
	for _, s := range Sites(f) {
		o := outcome[f.Units[s.Unit].Name]
		if o == "" {
			o = skipped
		}
		out = append(out, o)
	}
	return out
}
