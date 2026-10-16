package porttest

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// TestsImporting follows independently declared resolved adjacency. It reads
// every requested graph file to validate the actual inventory; declarations
// missing even an explicit no-edge entry are unavailable, never empty reach.
func (s *Inputs) TestsImporting(ctx context.Context, root string, sources, tests []string) (map[string][]string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return nil, failure
	}
	nodes := map[string]bool{}
	all := append(slices.Clone(sources), tests...)
	for _, path := range all {
		nodes[s.absolute(path)] = true
	}
	if s.Imports == nil {
		return nil, inputError(fmt.Errorf("import adjacency setup is unavailable"))
	}
	for path := range s.Imports {
		if !nodes[s.absolute(path)] {
			return nil, inputError(fmt.Errorf("declared import node %s is outside the requested inventory", path))
		}
	}
	for _, path := range all {
		if _, failure := s.required(ctx, path); failure != nil {
			return nil, failure
		}
		edges, known := s.Imports[s.absolute(path)]
		if !known {
			return nil, inputError(fmt.Errorf("no declared import adjacency for %s", path))
		}
		for _, target := range edges {
			if !nodes[s.absolute(target)] {
				return nil, inputError(fmt.Errorf("unknown import target %s from %s", target, path))
			}
		}
	}
	out := map[string][]string{}
	for _, source := range sources {
		out[source] = []string{}
	}
	for _, test := range tests {
		spec := lang.Detect(test)
		if spec == nil {
			continue
		}
		reached := map[string]bool{}
		if spec.Name == "go" {
			own := filepath.Dir(test)
			packages := map[string]bool{own: true}
			for _, target := range s.Imports[test] {
				packages[filepath.Dir(target)] = true
			}
			for _, source := range sources {
				if filepath.Dir(source) == own {
					for _, target := range s.Imports[source] {
						packages[filepath.Dir(target)] = true
					}
				}
			}
			for _, source := range sources {
				if packages[filepath.Dir(source)] {
					reached[source] = true
				}
			}
		} else {
			pending := []string{test}
			if spec.Name == "python" && (pythonFile(test) || filepath.Base(test) == "conftest.py") {
				top := s.nearest(ctx, test, "pyproject.toml", "setup.py", "setup.cfg")
				if top == "" {
					top = root
				}
				for dir := filepath.Dir(test); inside(top, dir); dir = filepath.Dir(dir) {
					conftest := filepath.Join(dir, "conftest.py")
					if conftest != test && nodes[conftest] {
						pending = append(pending, conftest)
					}
					if dir == top || filepath.Dir(dir) == dir {
						break
					}
				}
			}
			for len(pending) > 0 {
				path := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				if reached[path] {
					continue
				}
				reached[path] = true
				pending = append(pending, s.Imports[path]...)
			}
		}
		for _, source := range sources {
			if reached[source] {
				out[source] = append(out[source], test)
			}
		}
	}
	for source := range out {
		slices.Sort(out[source])
		out[source] = slices.Compact(out[source])
	}
	return out, inputStopped(ctx)
}
