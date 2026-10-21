package imports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTSConfigAliases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{
		// comment with "quotes" and a // inside
		"compilerOptions": { "baseUrl": "src", "paths": { "@app/*": ["app/*", "legacy/*"], "@app/core": ["core/index"], }, },
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := loadTSAliases(dir)
	rels := func(spec string) []string {
		var out []string
		for _, p := range a.resolve(spec) {
			r, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(r))
		}
		return out
	}
	cases := map[string][]string{
		"@app/button":  {"src/app/button", "src/legacy/button", "src/@app/button"},
		"@app/core":    {"src/core/index", "src/@app/core"},
		"utils/format": {"src/utils/format"},
	}
	for spec, want := range cases {
		if got := rels(spec); !slices.Equal(got, want) {
			t.Errorf("%s → %v, want %v", spec, got, want)
		}
	}
}

func TestStripJSONC(t *testing.T) {
	in := `{"a": "x // not a comment", /* gone */ "b": [1, 2,], // gone
}`
	var v map[string]any
	if err := json.Unmarshal(stripJSONC([]byte(in)), &v); err != nil || v["a"] != "x // not a comment" {
		t.Errorf("stripped %q: %v %v", stripJSONC([]byte(in)), v, err)
	}
}
