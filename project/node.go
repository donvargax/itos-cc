package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// nodeLockfiles name the package manager that wrote each lockfile.
var nodeLockfiles = []struct{ file, manager string }{
	{"pnpm-lock.yaml", "pnpm"}, {"pnpm-workspace.yaml", "pnpm"}, {"yarn.lock", "yarn"},
	{"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"},
}

// PackageManager is the package manager a Node project at dir declares: the
// packageManager field of the nearest package.json that has one, or the
// nearest lockfile, so a workspace member uses its root's. It is npm when
// nothing says otherwise.
func PackageManager(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if data, err := os.ReadFile(filepath.Join(d, "package.json")); err == nil {
			json.Unmarshal(data, &pkg)
		}
		if name, _, _ := strings.Cut(pkg.PackageManager, "@"); name != "" {
			return name
		}
		for _, l := range nodeLockfiles {
			if _, err := os.Stat(filepath.Join(d, l.file)); err == nil {
				return l.manager
			}
		}
		if filepath.Dir(d) == d {
			return "npm"
		}
	}
}

// NodeModule is the installed package name as dir resolves it, in the
// nearest node_modules at or above dir, or "" when it is not installed.
func NodeModule(dir, name string) string {
	return findUp(dir, filepath.Join("node_modules", filepath.FromSlash(name)))
}

// NodeBin is the installed executable name as dir resolves it, or "" when
// no package at or above dir installed one. Tools run from here rather than
// through npx, which downloads and runs whatever version the registry has
// when the project did not install one.
func NodeBin(dir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	return findUp(dir, filepath.Join("node_modules", ".bin", name))
}

func findUp(dir, rel string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if p := filepath.Join(d, rel); exists(p) {
			return p
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
