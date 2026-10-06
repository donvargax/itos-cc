package crap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/coverage"
)

// A route's lines count toward the route alone. mount ran when the module
// loaded, so it is fully covered even though no request reached its route.
func TestRouteLinesAreNotTheEnclosingFunctions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.ts")
	src := `export function mount(app) {
  const limit = 1;
  app.get("/x", (req, res) => {
    if (req.q > limit) { res.json(1); }
  });
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := coverage.ParseLCOV(strings.NewReader(
		"SF:" + path + "\nDA:2,1\nDA:3,1\nDA:4,0\nDA:5,1\nend_of_record\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Analyze([]string{path}, coverage.Build([]string{path}, filepath.Dir(path), entries))
	if err != nil {
		t.Fatal(err)
	}
	cov := map[string]float64{}
	for _, e := range got {
		cov[e.Name] = *e.Coverage
	}
	if cov["mount"] != 100 || cov["GET /x"] != 50 {
		t.Errorf("coverage = %v, want mount 100 and GET /x 50", cov)
	}
}
