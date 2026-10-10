package mutate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/donvargax/itos-cc/lang"
)

// LinkInstalledDependencies links into the frozen export the dependencies
// the live project has installed, which the commit never holds: the
// node_modules of each TypeScript build root and of every directory above
// it within the repository, as Node resolves them, and the .venv or venv of
// each Python build root, else the virtualenv VIRTUAL_ENV names. Each is a
// symbolic link to the live directory, so commands in the export and in
// workers' copies of it, which copy a link as a link, run the project's
// installed tools; nothing is installed or copied. Gradle's and Maven's
// local caches are outside every project and need no link. It returns the
// export-relative links it made. Run it once preparation has verified the
// export, whose Git status would count the links.
func (p *FreshPlan) LinkInstalledDependencies() ([]string, error) {
	if p == nil || p.FrozenRoot == "" || p.Root == "" {
		return nil, errors.New("linking installed dependencies needs a live frozen plan")
	}
	links := map[string]string{} // export-relative directory → live target
	for _, unit := range p.Units {
		path := filepath.Join(p.FrozenRoot, filepath.FromSlash(unit.Path))
		spec := lang.Detect(path)
		if spec == nil {
			continue
		}
		switch spec.Name {
		case "typescript":
			for dir := orDir(lang.FindUp(path, "package.json"), path); ; dir = filepath.Dir(dir) {
				rel, err := filepath.Rel(p.FrozenRoot, dir)
				if err != nil || !filepath.IsLocal(rel) {
					break
				}
				modules := filepath.Join(rel, "node_modules")
				if live := filepath.Join(p.Root, modules); isDir(live) {
					links[modules] = live
				}
				if rel == "." {
					break
				}
			}
		case "python":
			dir := orDir(lang.FindUp(path, "pyproject.toml", "setup.py", "setup.cfg"), path)
			rel, err := filepath.Rel(p.FrozenRoot, dir)
			if err != nil {
				continue
			}
			found := false
			for _, venv := range []string{".venv", "venv"} {
				if live := filepath.Join(p.Root, rel, venv); isDir(live) {
					links[filepath.Join(rel, venv)] = live
					found = true
				}
			}
			if env := os.Getenv("VIRTUAL_ENV"); !found && env != "" && fileExists(filepath.Join(env, "bin", "python")) {
				links[filepath.Join(rel, ".venv")] = env
			}
		}
	}
	var wanted, made []string
	for rel := range links {
		wanted = append(wanted, rel)
	}
	sort.Strings(wanted)
	for _, rel := range wanted {
		target := filepath.Join(p.FrozenRoot, rel)
		if _, err := os.Lstat(target); err == nil {
			// The commit holds it: the frozen one stands.
			continue
		}
		if err := os.Symlink(links[rel], target); err != nil {
			return made, fmt.Errorf("link installed dependencies %s: %w", filepath.ToSlash(rel), err)
		}
		made = append(made, filepath.ToSlash(rel))
	}
	return made, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
