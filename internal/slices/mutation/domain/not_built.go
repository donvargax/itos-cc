package domain

import (
	"context"
	"go/build/constraint"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
)

// Platform is the system a Go build is for: the GOOS and GOARCH go env
// reports in the project, never the system itos-cc was built for.
type Platform struct{ OS, Arch string }

// String is the platform as Go names it, "linux/amd64".
func (p Platform) String() string { return p.OS + "/" + p.Arch }

// NotBuilt is a source the build constraints of Platform leave out: only
// another system builds it, so no test here runs it, and its mutants are
// not judged here (slice-25, issue #18).
type NotBuilt struct {
	// Path is the file as the selection gave it.
	Path     string
	Platform Platform
}

// BuiltOn says whether Go builds the file named name, holding text, on p,
// as far as the platform decides it: its name's _GOOS or _GOARCH suffix
// and its //go:build line, as go/build reads them, rule it out on p
// whatever the other tags say. A file a tag of the project's own leaves
// out, integration or ignore, is built here as far as this rule goes: a
// build given that tag builds it.
func BuiltOn(name, text string, p Platform) bool {
	if nameRulesOut(name, p) {
		return false
	}
	expr := buildConstraint(text)
	return expr == nil || !ruledOut(expr, p)
}

// platformBound says whether the platform could leave the file out: its
// name or its //go:build line names a system or an architecture.
func platformBound(name, text string) bool {
	if _, _, ok := nameSuffix(name); ok {
		return true
	}
	bound := false
	if expr := buildConstraint(text); expr != nil {
		walkTags(expr, func(tag string) {
			bound = bound || knownOS[tag] || knownArch[tag] || tag == "unix"
		})
	}
	return bound
}

// SplitBuilt parts files into those their platform builds, in their order,
// and the Go files only another system builds (BuiltOn). The platform is
// the one tools reports in each file's folder, asked only for a file whose
// name or constraint names a system or an architecture, once a folder. A
// file around cannot read is built, as its parse then says why.
func SplitBuilt(ctx context.Context, files []string, around langport.Surroundings, tools Toolchain) ([]string, []NotBuilt, error) {
	var built []string
	var notBuilt []NotBuilt
	platforms := map[string]Platform{}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, nil, context.Cause(ctx)
		}
		spec := lang.Detect(path)
		if spec == nil || spec.Name != "go" {
			built = append(built, path)
			continue
		}
		data, ok := around.Read(ctx, path)
		name := filepath.Base(path)
		if !ok || !platformBound(name, string(data)) {
			built = append(built, path)
			continue
		}
		dir := filepath.Dir(path)
		p, seen := platforms[dir]
		if !seen {
			var failure InputFailure
			if p, failure = tools.GoPlatform(ctx, dir); failure != nil {
				return nil, nil, failure
			}
			platforms[dir] = p
		}
		if BuiltOn(name, string(data), p) {
			built = append(built, path)
			continue
		}
		notBuilt = append(notBuilt, NotBuilt{Path: path, Platform: p})
	}
	return built, notBuilt, nil
}

// nameSuffix is the GOOS and GOARCH a file name's suffix names, either ""
// when it names none, as go/build's goodOSArchFile reads it: before the
// extension and any _test, the last part after an underscore, or the last
// two, *_GOOS, *_GOARCH or *_GOOS_GOARCH; what comes before the first
// underscore never counts.
func nameSuffix(name string) (goos, goarch string, ok bool) {
	name, _, _ = strings.Cut(name, ".")
	i := strings.Index(name, "_")
	if i < 0 {
		return "", "", false
	}
	l := strings.Split(name[i:], "_")
	if n := len(l); n > 0 && l[n-1] == "test" {
		l = l[:n-1]
	}
	n := len(l)
	switch {
	case n >= 2 && knownOS[l[n-2]] && knownArch[l[n-1]]:
		return l[n-2], l[n-1], true
	case n >= 1 && knownOS[l[n-1]]:
		return l[n-1], "", true
	case n >= 1 && knownArch[l[n-1]]:
		return "", l[n-1], true
	}
	return "", "", false
}

// nameRulesOut says whether a file name's suffix rules out p.
func nameRulesOut(name string, p Platform) bool {
	goos, goarch, ok := nameSuffix(name)
	if !ok {
		return false
	}
	return goos != "" && !matchOS(goos, p.OS) || goarch != "" && goarch != p.Arch
}

// buildConstraint is the file's build constraint, nil when it has none or
// it does not parse: its //go:build line, else its // +build lines joined,
// among the line comments before the package clause that end with a blank
// line, as go/build reads them.
func buildConstraint(text string) constraint.Expr {
	var header []string
	end := 0
	inBlock := false
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		switch {
		case inBlock:
			if _, after, closed := strings.Cut(trimmed, "*/"); closed {
				inBlock = false
				if strings.TrimSpace(after) != "" {
					return parseHeader(header[:end])
				}
			}
		case trimmed == "":
			end = len(header)
		case strings.HasPrefix(trimmed, "//"):
		case strings.HasPrefix(trimmed, "/*"):
			inBlock = !strings.Contains(trimmed[2:], "*/")
		default:
			return parseHeader(header[:end])
		}
		header = append(header, trimmed)
	}
	return parseHeader(header[:end])
}

// parseHeader is the constraint lines hold: the first //go:build line,
// else every // +build line, and.
func parseHeader(lines []string) constraint.Expr {
	var plus constraint.Expr
	for _, line := range lines {
		if constraint.IsGoBuild(line) {
			expr, err := constraint.Parse(line)
			if err != nil {
				return nil
			}
			return expr
		}
		if constraint.IsPlusBuild(line) {
			expr, err := constraint.Parse(line)
			if err != nil {
				return nil
			}
			if plus == nil {
				plus = expr
			} else {
				plus = &constraint.AndExpr{X: plus, Y: expr}
			}
		}
	}
	return plus
}

// freeLimit bounds the tags a constraint is judged over unset: one naming
// more is taken as built.
const freeLimit = 12

// ruledOut says whether p rules expr out: false on p however the other
// tags are set, and true on some platform. One true on none, as
// "integration && !integration", no platform rules out.
func ruledOut(expr constraint.Expr, p Platform) bool {
	onP := func(tag string) (bool, bool) { return platformTag(tag, p) }
	anywhere := func(string) (bool, bool) { return false, false }
	return !satisfiable(expr, onP) && satisfiable(expr, anywhere)
}

// satisfiable says whether expr is true for some setting of the tags
// fixed leaves unset; with more than freeLimit of them, it is taken to be.
func satisfiable(expr constraint.Expr, fixed func(tag string) (value, ok bool)) bool {
	index := map[string]int{}
	walkTags(expr, func(tag string) {
		if _, ok := fixed(tag); !ok {
			if _, seen := index[tag]; !seen {
				index[tag] = len(index)
			}
		}
	})
	if len(index) > freeLimit {
		return true
	}
	for set := 0; set < 1<<len(index); set++ {
		if expr.Eval(func(tag string) bool {
			if value, ok := fixed(tag); ok {
				return value
			}
			return set&(1<<index[tag]) != 0
		}) {
			return true
		}
	}
	return false
}

// walkTags calls each with every tag expr names.
func walkTags(expr constraint.Expr, each func(tag string)) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		each(e.Tag)
	case *constraint.NotExpr:
		walkTags(e.X, each)
	case *constraint.AndExpr:
		walkTags(e.X, each)
		walkTags(e.Y, each)
	case *constraint.OrExpr:
		walkTags(e.X, each)
		walkTags(e.Y, each)
	}
}

// platformTag is whether tag holds on p, fixed when the platform decides
// it: a system, "unix", or an architecture.
func platformTag(tag string, p Platform) (value, fixed bool) {
	switch {
	case knownOS[tag]:
		return matchOS(tag, p.OS), true
	case tag == "unix":
		return unixOS[p.OS], true
	case knownArch[tag]:
		return tag == p.Arch, true
	}
	return false, false
}

// matchOS says whether the system tag holds on goos: its own name, and
// linux on android, solaris on illumos and darwin on ios, as Go sets them.
func matchOS(tag, goos string) bool {
	return tag == goos || goos == "android" && tag == "linux" || goos == "illumos" && tag == "solaris" || goos == "ios" && tag == "darwin"
}

// knownOS, unixOS and knownArch are Go's lists of systems, of the Unix
// ones among them, and of architectures (go/build's syslist.go).
var (
	knownOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js", "linux",
		"nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos")
	unixOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "linux",
		"netbsd", "openbsd", "solaris")
	knownArch = setOf("386", "amd64", "amd64p32", "arm", "armbe", "arm64", "arm64be", "loong64", "mips", "mipsle",
		"mips64", "mips64le", "mips64p32", "mips64p32le", "ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390",
		"s390x", "sparc", "sparc64", "wasm")
)

func setOf(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
