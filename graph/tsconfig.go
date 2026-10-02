package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// tsAliases are the module aliases of a tsconfig.json: `paths` patterns such
// as "@/*" → ["./src/*"], and `baseUrl`, which makes "components/button"
// mean src/components/button.
type tsAliases struct {
	baseURL string // absolute, or "" when unset
	paths   []tsPath
}

type tsPath struct {
	prefix, suffix string
	wildcard       bool
	targets        []string // absolute, with the * still in place
}

type tsconfig struct {
	Extends         any `json:"extends"`
	CompilerOptions struct {
		BaseURL *string             `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
	References []struct {
		Path string `json:"path"`
	} `json:"references"`
}

// loadTSAliases reads dir/tsconfig.json with what it extends. A solution
// config that only lists references, as Vite's templates write, takes its
// aliases from the first referenced config that declares some.
func loadTSAliases(dir string) *tsAliases {
	a := &tsAliases{}
	root := filepath.Join(dir, "tsconfig.json")
	a.load(root, map[string]bool{})
	if a.baseURL == "" && len(a.paths) == 0 {
		var cfg tsconfig
		if readJSONC(root, &cfg) {
			for _, ref := range cfg.References {
				p := filepath.Join(dir, ref.Path)
				if info, err := os.Stat(p); err == nil && info.IsDir() {
					p = filepath.Join(p, "tsconfig.json")
				}
				a.load(p, map[string]bool{})
				if a.baseURL != "" || len(a.paths) > 0 {
					break
				}
			}
		}
	}
	return a
}

// load applies file over what it extends: an extending config's baseUrl and
// paths replace the base's, as in TypeScript.
func (a *tsAliases) load(file string, seen map[string]bool) {
	if seen[file] {
		return
	}
	seen[file] = true
	var cfg tsconfig
	if !readJSONC(file, &cfg) {
		return
	}
	dir := filepath.Dir(file)
	if base, ok := cfg.Extends.(string); ok && strings.HasPrefix(base, ".") {
		p := filepath.Join(dir, base)
		if !strings.HasSuffix(p, ".json") {
			p += ".json"
		}
		a.load(p, seen)
	}
	if cfg.CompilerOptions.BaseURL != nil {
		a.baseURL = filepath.Join(dir, *cfg.CompilerOptions.BaseURL)
	}
	if cfg.CompilerOptions.Paths != nil {
		// Targets resolve against baseUrl when set, else against the config
		// that declares them.
		from := dir
		if a.baseURL != "" {
			from = a.baseURL
		}
		a.paths = nil
		for pattern, targets := range cfg.CompilerOptions.Paths {
			p := tsPath{}
			p.prefix, p.suffix, p.wildcard = strings.Cut(pattern, "*")
			for _, t := range targets {
				p.targets = append(p.targets, filepath.Join(from, t))
			}
			a.paths = append(a.paths, p)
		}
		// The longest prefix wins, as TypeScript matches.
		sort.Slice(a.paths, func(i, j int) bool { return len(a.paths[i].prefix) > len(a.paths[j].prefix) })
	}
}

// resolve returns the absolute paths, without extension, that spec may name.
func (a *tsAliases) resolve(spec string) []string {
	var out []string
	for _, p := range a.paths {
		var star string
		switch {
		case !p.wildcard && spec == p.prefix:
		case p.wildcard && strings.HasPrefix(spec, p.prefix) && strings.HasSuffix(spec, p.suffix) &&
			len(spec) >= len(p.prefix)+len(p.suffix):
			star = spec[len(p.prefix) : len(spec)-len(p.suffix)]
		default:
			continue
		}
		for _, t := range p.targets {
			out = append(out, strings.Replace(t, "*", star, 1))
		}
		break
	}
	if a.baseURL != "" {
		out = append(out, filepath.Join(a.baseURL, spec))
	}
	return out
}

// readJSONC reads JSON with comments and trailing commas, which tsconfig
// files allow.
func readJSONC(file string, v any) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	return json.Unmarshal(stripJSONC(data), v) == nil
}

func stripJSONC(src []byte) []byte {
	return dropTrailingCommas(dropComments(src))
}

// scanJSON calls emit for each byte outside strings, and copies strings
// through untouched. emit returns how many extra bytes it consumed.
func scanJSON(src []byte, emit func(i int, out *[]byte) int) []byte {
	out := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		if src[i] != '"' {
			i += emit(i, &out)
			continue
		}
		start := i
		for i++; i < len(src) && src[i] != '"'; i++ {
			if src[i] == '\\' {
				i++
			}
		}
		out = append(out, src[start:min(i+1, len(src))]...)
	}
	return out
}

func dropComments(src []byte) []byte {
	return scanJSON(src, func(i int, out *[]byte) int {
		switch {
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '/':
			end := i
			for end < len(src) && src[end] != '\n' {
				end++
			}
			return end - i - 1
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '*':
			end := i + 2
			for end+1 < len(src) && !(src[end] == '*' && src[end+1] == '/') {
				end++
			}
			*out = append(*out, ' ')
			return end + 1 - i
		}
		*out = append(*out, src[i])
		return 0
	})
}

// dropTrailingCommas removes a comma that only whitespace separates from }
// or ].
func dropTrailingCommas(src []byte) []byte {
	return scanJSON(src, func(i int, out *[]byte) int {
		if src[i] == ',' {
			j := i + 1
			for j < len(src) && strings.ContainsRune(" \t\r\n", rune(src[j])) {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				return 0
			}
		}
		*out = append(*out, src[i])
		return 0
	})
}
