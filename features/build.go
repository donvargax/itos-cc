package features

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
)

// The binary driver's builds of itos-cc, as a release, go install and a
// person's go build make them, for the scenarios about what a build says
// it is (cli.feature's ID-CLI-03). Each builds into the scenario's folder,
// with -trimpath, which changes nothing a build records of its version and
// lets it reuse what the harness's own build compiled.

// Build builds itos-cc from the module at b.root in dir as how says.
func (b binary) Build(dir string, how Build) (Built, error) {
	bin := filepath.Join(dir, "bin", "itos-cc")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	switch how.How {
	case "release":
		// A release stamps main.version; its build reads no git.
		err := goCommand(b.root, nil, "build", "-trimpath", "-buildvcs=false",
			"-ldflags", "-X main.version="+how.Version, "-o", bin, "./cmd/itos-cc")
		return Built{Binary: bin}, err
	case "install":
		return b.install(dir, how.Version, filepath.Dir(bin))
	case "checkout":
		return b.checkout(dir, how.Version, bin)
	case "no-git":
		tree := filepath.Join(dir, "tree")
		for d := tree; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
				return Built{}, fmt.Errorf("%s is a repository above the scratch folder: %w", d, godog.ErrSkip)
			}
			if filepath.Dir(d) == d {
				break
			}
		}
		if err := b.copyModule(tree); err != nil {
			return Built{}, err
		}
		err := goCommand(tree, nil, "build", "-trimpath", "-o", bin, "./cmd/itos-cc")
		return Built{Binary: bin}, err
	}
	return Built{}, fmt.Errorf("no build %q", how.How)
}

// install serves the module at version from a module proxy in dir, the
// tree's files zipped as the go command zips a module, its dependencies
// from the module cache's download folder, and go installs it into bin
// with a module cache of its own, which it then cleans.
func (b binary) install(dir, version, bin string) (Built, error) {
	module, err := modulePath(b.root)
	if err != nil {
		return Built{}, err
	}
	proxy := filepath.Join(dir, "proxy")
	at := filepath.Join(proxy, filepath.FromSlash(strings.ToLower(module)), "@v")
	if err := os.MkdirAll(at, 0o755); err != nil {
		return Built{}, err
	}
	if err := b.zipModule(filepath.Join(at, version+".zip"), module+"@"+version); err != nil {
		return Built{}, err
	}
	mod, err := os.ReadFile(filepath.Join(b.root, "go.mod"))
	if err != nil {
		return Built{}, err
	}
	for name, text := range map[string]string{
		version + ".mod":  string(mod),
		version + ".info": `{"Version":"` + version + `","Time":"2026-01-01T00:00:00Z"}`,
		"list":            version + "\n",
	} {
		if err := os.WriteFile(filepath.Join(at, name), []byte(text), 0o600); err != nil {
			return Built{}, err
		}
	}
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		return Built{}, fmt.Errorf("go env GOMODCACHE: %v", err)
	}
	downloads := filepath.Join(strings.TrimSpace(string(out)), "cache", "download")
	cache := filepath.Join(dir, "modcache")
	env := []string{
		"GOPROXY=" + fileURL(proxy) + "," + fileURL(downloads), "GOSUMDB=off", "GOFLAGS=-trimpath",
		"GOMODCACHE=" + cache, "GOTOOLCHAIN=local", "GOWORK=off", "GOBIN=" + bin,
	}
	err = goCommand(dir, env, "install", module+"/cmd/itos-cc@"+version)
	// The module cache's files are read-only: cleaned, the scratch folder
	// can be removed.
	_ = goCommand(dir, env, "clean", "-modcache")
	name := "itos-cc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return Built{Binary: filepath.Join(bin, name)}, err
}

// checkout copies the tree into a git repository in dir, whose first
// commit is tagged version and whose second, the one built, comes after
// it, and builds it there with go build, which records the version git
// gives it.
func (b binary) checkout(dir, version, bin string) (Built, error) {
	tree := filepath.Join(dir, "checkout")
	if err := b.copyModule(tree); err != nil {
		return Built{}, err
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "the release"}, {"tag", version},
		{"commit", "-q", "--allow-empty", "-m", "after the release"},
	} {
		if err := b.Git(tree, args...); err != nil {
			return Built{}, err
		}
	}
	git, err := realGit()
	if err != nil {
		return Built{}, err
	}
	cmd := exec.Command(git, "rev-parse", "HEAD")
	cmd.Dir, cmd.Env = tree, environment(nil)
	out, err := cmd.Output()
	if err != nil {
		return Built{}, fmt.Errorf("git rev-parse HEAD: %v", err)
	}
	err = goCommand(tree, nil, "build", "-trimpath", "-o", bin, "./cmd/itos-cc")
	return Built{Binary: bin, Commit: strings.TrimSpace(string(out))}, err
}

// goCommand runs go with args in dir, in the scenarios' environment with
// vars set.
func goCommand(dir string, vars []string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Env = dir, environment(vars)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// fileURL is the file URL of the absolute path p.
func fileURL(p string) string {
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

// modulePath is the path go.mod at root declares.
func modulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(lines.Text()), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", errors.New("go.mod declares no module")
}

// moduleFiles are the files of the module at root a build needs, by
// slash-separated path: go.mod, go.sum and each Go source that is no test,
// outside hidden folders, node_modules, vendor, testdata and folders of
// other modules.
func (b binary) moduleFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(b.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(b.root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "go.mod" || rel == "go.sum" || strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// copyModule copies the module's files into dir.
func (b binary) copyModule(dir string) error {
	files, err := b.moduleFiles()
	if err != nil {
		return err
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if err := b.Write(filepath.Join(dir, filepath.FromSlash(rel)), data); err != nil {
			return err
		}
	}
	return nil
}

// zipModule writes the module's files to name as a module zip, each under
// prefix, its path and version.
func (b binary) zipModule(name, prefix string) error {
	files, err := b.moduleFiles()
	if err != nil {
		return err
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(rel)))
		if err != nil {
			_ = f.Close()
			return err
		}
		w, err := z.Create(prefix + "/" + rel)
		if err == nil {
			_, err = w.Write(data)
		}
		if err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := z.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Occupy listens on a free port of 127.0.0.1 until release.
func (binary) Occupy() (int, func(), error) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	addr := held.Addr().String()
	if second, err := net.Listen("tcp", addr); err == nil {
		_ = second.Close()
		_ = held.Close()
		return 0, nil, fmt.Errorf("%w: the system let a second listener take %s", godog.ErrSkip, addr)
	}
	return held.Addr().(*net.TCPAddr).Port, func() { _ = held.Close() }, nil
}
