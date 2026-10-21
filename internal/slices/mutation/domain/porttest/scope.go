package porttest

// This in-memory boundary reads the same accepted declaration syntax from actual
// Files and Links. It never follows a dependency target or scripts a verdict.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// LineCoverageUnsupported names the scope of source, a file of a
// line-precision language, that its strict coverage fingerprints cannot
// prove, or "" when there is none (q-40, as ADR-0019 refuses Go's):
//   - TypeScript: an npm, yarn or pnpm workspace its package belongs to (a
//     "workspaces" field in a package.json from its package root up to the
//     project root, or a pnpm-workspace.yaml there), a workspace: dependency,
//     or a file: or link: dependency outside its package root's inventory.
//   - Python: a path or editable dependency outside its build root's
//     inventory, as pyproject.toml names one (a path = "…" key, such as
//     [tool.uv.sources] or Poetry's, or a file: URL), as a requirements file
//     there does (-e or --editable <path>, a path, a file: URL), or as the
//     editable installs of the root's virtualenv point at one (a .pth line, a
//     setuptools __editable__ finder's mapping).
//   - Kotlin: a Gradle includeBuild outside the build root, in its settings
//     script, or a Maven <module>, <subproject> or parent <relativePath>
//     outside it, in the pom.xml files from the module up to the reactor's
//     top.
//
// A target is outside the inventory when it resolves outside the root, or
// through a symbolic link or a nested build root (another package, Python
// build root) beneath it, which the fingerprints leave out. Detection is
// lexical: it reads only files beneath projectRoot, and never follows,
// opens or lists a target outside it.
// scopeReader reads the files a scope is declared in, beneath the project
// root alone.
type scopeReader struct {
	root   string
	inputs *Inputs
}

// read is the file at path, "" when it is not beneath the root or cannot
// be read.
func (s scopeReader) read(ctx context.Context, path string) string {
	if !inside(s.root, path) {
		return ""
	}
	data, ok := s.inputs.Read(ctx, path)
	if !ok {
		return ""
	}
	return string(data)
}

// rel is path as a message names it: from the project root, with forward
// slashes.
func (s scopeReader) rel(path string) string {
	if rel, err := filepath.Rel(s.root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// outside says whether target, a path as a file beneath base declares it,
// resolves outside the inventory of the build root at root: outside root,
// or through a symbolic link or a directory nested says is another build
// root. Only paths beneath root are inspected, with Lstat.
func (s scopeReader) outside(ctx context.Context, root, base, target string, nested func(dir string) bool) bool {
	if rooted(target) && !filepath.IsAbs(target) {
		// On Windows, \x or /x is on the current drive and C:x relative to
		// C:'s working directory: neither is beneath the root.
		return true
	}
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	if !inside(root, path) || !inside(s.root, path) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	if rel == "." {
		return false
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		if _, linked := s.inputs.Links[current]; linked {
			return true
		}
		if s.inputs.Exists(ctx, current) && nested != nil && nested(current) {
			return true
		}
	}
	return false
}

// rooted says whether target, as a file declares it, is not relative to
// that file: absolute, or, as Windows reads it, rooted on a drive or with
// a drive and no root.
func rooted(target string) bool {
	return filepath.IsAbs(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, `\`) ||
		filepath.VolumeName(target) != ""
}

// buildRoot is the nearest directory above source holding one of markers,
// "" when there is none beneath the project root.
func (s scopeReader) buildRoot(ctx context.Context, source string, markers ...string) string {
	dir := s.inputs.nearest(ctx, source, markers...)
	if dir == "" || !inside(s.root, dir) {
		return ""
	}
	return dir
}

// typescriptDependencyFields are the package.json fields that name
// dependencies.
var typescriptDependencyFields = []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}

func (s scopeReader) typescript(ctx context.Context, source string) string {
	pkg := s.buildRoot(ctx, source, "package.json")
	if pkg == "" {
		return ""
	}
	for dir := pkg; ; dir = filepath.Dir(dir) {
		if workspace := filepath.Join(dir, "pnpm-workspace.yaml"); s.inputs.regular(ctx, workspace) {
			return "pnpm workspace (" + s.rel(workspace) + ")"
		}
		if text := s.read(ctx, filepath.Join(dir, "package.json")); text != "" {
			var manifest map[string]json.RawMessage
			if json.Unmarshal([]byte(text), &manifest) == nil {
				if raw, ok := manifest["workspaces"]; ok && string(raw) != "null" {
					return "npm or yarn workspace (" + s.rel(filepath.Join(dir, "package.json")) + " workspaces)"
				}
			}
		}
		if dir == s.root || filepath.Dir(dir) == dir || !inside(s.root, filepath.Dir(dir)) {
			break
		}
	}
	manifest := filepath.Join(pkg, "package.json")
	var deps map[string]map[string]string
	if json.Unmarshal([]byte(s.read(ctx, manifest)), &deps) != nil {
		// A field that is not a map of strings fails the whole decode: read
		// each field alone.
		deps = map[string]map[string]string{}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(s.read(ctx, manifest)), &fields)
		for _, field := range typescriptDependencyFields {
			var m map[string]string
			if json.Unmarshal(fields[field], &m) == nil {
				deps[field] = m
			}
		}
	}
	nestedPackage := func(dir string) bool { return s.inputs.regular(ctx, filepath.Join(dir, "package.json")) }
	for _, field := range typescriptDependencyFields {
		names := make([]string, 0, len(deps[field]))
		for name := range deps[field] {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			spec := deps[field][name]
			switch {
			case strings.HasPrefix(spec, "workspace:"):
				return fmt.Sprintf("workspace dependency %s %q in %s", name, spec, s.rel(manifest))
			case strings.HasPrefix(spec, "file:"), strings.HasPrefix(spec, "link:"):
				target := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(spec, "file:"), "link:"), "//")
				if s.outside(ctx, pkg, pkg, target, nestedPackage) || !s.inputs.directory(ctx, filepath.Join(pkg, filepath.FromSlash(target))) {
					return fmt.Sprintf("local dependency %s %q in %s, outside the package root", name, spec, s.rel(manifest))
				}
			}
		}
	}
	return ""
}

var (
	// pyprojectPath is a path key of pyproject.toml, as [tool.uv.sources]
	// and Poetry's dependencies write a path dependency.
	pyprojectPath = regexp.MustCompile(`(?m)\bpath\s*=\s*["']([^"']+)["']`)
	// fileURL is a file: URL in a requirement.
	fileURL = regexp.MustCompile(`file:(?://)?([^"'\s;,\]]+)`)
	// editableMapping is the mapping of a setuptools __editable__ finder.
	editableMapping = regexp.MustCompile(`(?s)MAPPING\s*(?::[^=]*)?=\s*\{(.*?)\}`)
	quoted          = regexp.MustCompile(`'([^']*)'|"([^"]*)"`)
)

func (s scopeReader) python(ctx context.Context, source string) string {
	root := s.buildRoot(ctx, source, pythonRootMarkers...)
	if root == "" {
		return ""
	}
	nestedRoot := func(dir string) bool {
		for _, marker := range pythonRootMarkers {
			if s.inputs.regular(ctx, filepath.Join(dir, marker)) {
				return true
			}
		}
		return false
	}
	refuse := func(target, file string) string {
		if s.outside(ctx, root, root, target, nestedRoot) {
			return fmt.Sprintf("path dependency %q in %s, outside the build root", target, s.rel(file))
		}
		return ""
	}
	pyproject := filepath.Join(root, "pyproject.toml")
	text := s.read(ctx, pyproject)
	for _, m := range pyprojectPath.FindAllStringSubmatch(text, -1) {
		if why := refuse(m[1], pyproject); why != "" {
			return why
		}
	}
	for _, m := range fileURL.FindAllStringSubmatch(text, -1) {
		if why := refuse(m[1], pyproject); why != "" {
			return why
		}
	}
	requirements, _ := s.inputs.glob(filepath.Join(root, "requirements*.txt"))
	slices.Sort(requirements)
	for _, file := range requirements {
		for _, line := range strings.Split(s.read(ctx, file), "\n") {
			if target := requirementPath(line); target != "" {
				if why := refuse(target, file); why != "" {
					return why
				}
			}
		}
	}
	for _, venv := range []string{".venv", "venv"} {
		sites, _ := s.inputs.glob(filepath.Join(root, venv, "lib", "python*", "site-packages"))
		sites2, _ := s.inputs.glob(filepath.Join(root, venv, "Lib", "site-packages"))
		for _, site := range append(sites, sites2...) {
			pths, _ := s.inputs.glob(filepath.Join(site, "*.pth"))
			finders, _ := s.inputs.glob(filepath.Join(site, "__editable__*finder.py"))
			slices.Sort(pths)
			slices.Sort(finders)
			for _, pth := range pths {
				for _, line := range strings.Split(s.read(ctx, pth), "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import\t") {
						continue
					}
					if s.outside(ctx, root, site, line, nil) {
						return fmt.Sprintf("editable install %q in %s, outside the build root", line, s.rel(pth))
					}
				}
			}
			for _, finder := range finders {
				for _, mapping := range editableMapping.FindAllStringSubmatch(s.read(ctx, finder), -1) {
					for _, q := range quoted.FindAllStringSubmatch(mapping[1], -1) {
						target := q[1] + q[2]
						if rooted(target) && s.outside(ctx, root, root, target, nil) {
							return fmt.Sprintf("editable install %q in %s, outside the build root", target, s.rel(finder))
						}
					}
				}
			}
		}
	}
	return ""
}

// requirementPath is the local path a requirements file line installs:
// that of -e or --editable, a line that is a path, or a file: URL; "" for
// a requirement by name or a remote URL.
func requirementPath(line string) string {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, " #"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	for _, flag := range []string{"-e", "--editable"} {
		if rest, ok := strings.CutPrefix(line, flag); ok && (strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, "\t")) {
			line = strings.TrimSpace(strings.TrimLeft(rest, "= \t"))
			break
		}
	}
	if strings.HasPrefix(line, "file:") || strings.Contains(line, "@ file:") {
		if m := fileURL.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	if strings.Contains(line, "://") || strings.HasPrefix(line, "git+") {
		return ""
	}
	if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "/") || filepath.IsAbs(line) {
		if i := strings.IndexAny(line, "[ ;"); i > 0 {
			line = line[:i]
		}
		return line
	}
	return ""
}

var (
	// includeBuild is a Gradle settings script's includeBuild of a path.
	includeBuild = regexp.MustCompile(`includeBuild\s*\(?\s*(?:file\(\s*)?["']([^"']+)["']`)
	// mavenModule is a pom.xml's <module> or <subproject>.
	mavenModule = regexp.MustCompile(`<(module|subproject)>\s*([^<\s]+)\s*</(?:module|subproject)>`)
	// relativePath is a pom.xml parent's <relativePath>.
	relativePath = regexp.MustCompile(`<relativePath>\s*([^<\s]+)\s*</relativePath>`)
	// xmlComment is a comment of an XML file.
	xmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func (s scopeReader) kotlin(ctx context.Context, source string) string {
	module := s.buildRoot(ctx, source, kotlinModuleMarkers...)
	if module == "" {
		return ""
	}
	if !s.inputs.regular(ctx, filepath.Join(module, "pom.xml")) {
		root := s.buildRoot(ctx, source, kotlinSettings...)
		if root == "" {
			return ""
		}
		for _, name := range kotlinSettings {
			settings := filepath.Join(root, name)
			for _, m := range includeBuild.FindAllStringSubmatch(s.read(ctx, settings), -1) {
				if s.outside(ctx, root, root, m[1], nil) {
					return fmt.Sprintf("includeBuild(%q) in %s, outside the build root", m[1], s.rel(settings))
				}
			}
		}
		return ""
	}
	var poms []string
	top := module
	for dir := module; inside(s.root, dir) && s.inputs.regular(ctx, filepath.Join(dir, "pom.xml")); dir = filepath.Dir(dir) {
		poms = append(poms, filepath.Join(dir, "pom.xml"))
		top = dir
		if filepath.Dir(dir) == dir {
			break
		}
	}
	for i := len(poms) - 1; i >= 0; i-- {
		pom := poms[i]
		text := xmlComment.ReplaceAllString(s.read(ctx, pom), "")
		for _, m := range mavenModule.FindAllStringSubmatch(text, -1) {
			if s.outside(ctx, top, filepath.Dir(pom), m[2], nil) {
				return fmt.Sprintf("<%s>%s</%s> in %s, outside the build root", m[1], m[2], m[1], s.rel(pom))
			}
		}
		for _, m := range relativePath.FindAllStringSubmatch(text, -1) {
			if s.outside(ctx, top, filepath.Dir(pom), m[1], nil) {
				return fmt.Sprintf("parent <relativePath>%s</relativePath> in %s, outside the build root", m[1], s.rel(pom))
			}
		}
	}
	return ""
}

var pythonRootMarkers = []string{"pyproject.toml", "setup.py", "setup.cfg"}
var kotlinModuleMarkers = []string{"build.gradle.kts", "build.gradle", "pom.xml"}
var kotlinSettings = []string{"settings.gradle.kts", "settings.gradle"}

func (s *Inputs) regular(ctx context.Context, path string) bool {
	if ctx.Err() != nil {
		return false
	}
	_, ok := s.Files[s.absolute(path)]
	return ok
}
func (s *Inputs) directory(ctx context.Context, path string) bool {
	return s.Exists(ctx, path) && !s.regular(ctx, path)
}
func (s *Inputs) glob(pattern string) ([]string, error) {
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, err
	}
	entries := map[string]bool{}
	for path := range s.Files {
		for p := path; ; p = filepath.Dir(p) {
			entries[p] = true
			if filepath.Dir(p) == p {
				break
			}
		}
	}
	var found []string
	for path := range entries {
		if ok, _ := filepath.Match(pattern, path); ok {
			found = append(found, path)
		}
	}
	slices.Sort(found)
	return found, nil
}

func (s *Inputs) Unsupported(ctx context.Context, language, source, root string) (string, mutants.InputFailure) {
	if failure := inputStopped(ctx); failure != nil {
		return "", failure
	}
	if language == "go" {
		return s.goUnsupported(ctx, source)
	}
	reader := scopeReader{root: s.absolute(root), inputs: s}
	var reason string
	switch language {
	case "typescript":
		reason = reader.typescript(ctx, s.absolute(source))
	case "python":
		reason = reader.python(ctx, s.absolute(source))
	case "kotlin":
		reason = reader.kotlin(ctx, s.absolute(source))
	}
	return reason, inputStopped(ctx)
}
