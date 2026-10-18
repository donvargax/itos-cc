package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/cucumber/godog"
)

func TestBinaryMissingC8FacadeIsolation(t *testing.T) {
	original := t.TempDir()
	dest := filepath.Join(t.TempDir(), "node_modules")
	files := map[string]string{
		"c8/package.json": "collector", "vitest/package.json": "runner", "@vitest/coverage-v8/package.json": "provider",
		".bin/c8": "c8", ".bin/c8.cmd": "c8 cmd", ".bin/c8.ps1": "c8 ps1", ".bin/vitest": "vitest", ".bin/vitest.cmd": "vitest cmd",
	}
	d := binary{}
	for file, text := range files {
		if err := d.Write(filepath.Join(original, filepath.FromSlash(file)), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := coverageWithoutC8(original, dest, runtime.GOOS, false); errors.Is(err, godog.ErrSkip) {
		t.Skip(err)
	} else if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"c8", ".bin/c8", ".bin/c8.cmd", ".bin/c8.ps1"} {
		if _, err := os.Lstat(filepath.Join(dest, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("excluded collector %s: %v", name, err)
		}
	}
	for file, text := range files {
		originalBytes, err := d.Read(filepath.Join(original, filepath.FromSlash(file)))
		if err != nil || string(originalBytes) != text {
			t.Fatalf("original %s changed: %q %v", file, originalBytes, err)
		}
		if file == "c8/package.json" || file == ".bin/c8" || file == ".bin/c8.cmd" || file == ".bin/c8.ps1" {
			continue
		}
		got, err := d.Read(filepath.Join(dest, filepath.FromSlash(file)))
		if err != nil || string(got) != text {
			t.Fatalf("retained %s: %q %v", file, got, err)
		}
	}
	if err := coverageWithoutC8(original, original, runtime.GOOS, false); err == nil || errors.Is(err, godog.ErrSkip) {
		t.Fatalf("existing destination was not an error: %v", err)
	}
	if err := coverageWithoutC8(filepath.Join(original, "missing"), filepath.Join(t.TempDir(), "facade"), runtime.GOOS, false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary read failure: %v", err)
	}
}

func TestBinaryIntegrationDeclarations(t *testing.T) { IntegrationDeclarationContract(t, binary{}) }

func TestBinaryMissingC8FacadeLinkFailures(t *testing.T) {
	for _, test := range []struct {
		name, platform string
		cause          error
		omit           bool
	}{
		{"unsupported", "linux", errors.ErrUnsupported, true},
		{"permission", "linux", os.ErrPermission, true},
		{"unimplemented", "linux", syscall.ENOSYS, true},
		{"windows privilege", "windows", syscall.Errno(1314), true},
		{"foreign errno", "linux", syscall.Errno(1314), false},
		{"full disk", "linux", syscall.ENOSPC, false},
		{"missing path", "linux", os.ErrNotExist, false},
		{"existing path", "linux", os.ErrExist, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			link := &os.LinkError{Op: "symlink", Old: "owned-source", New: "owned-target", Err: test.cause}
			for _, required := range []bool{false, true} {
				err := coverageLinkFailure(test.platform, required, link)
				if errors.Is(err, godog.ErrSkip) != (test.omit && !required) || !errors.Is(err, test.cause) {
					t.Fatalf("required %v cause %v: %v", required, test.cause, err)
				}
				if (required || !test.omit) && err != link {
					t.Fatalf("ordinary/required failure replaced: %v", err)
				}
			}
		})
	}
}

// TestBinaryNodeModulesKeepToolCachesApart holds what scenarios running at
// once need of the node_modules Coverage links (T-128), which no scenario
// observes: each scenario's own folder of links to the pinned packages, so
// Vitest's .vite and the tools' .cache are made in the scenario's folder,
// never written into the installation every scenario shares, and a second
// declaration in the same folder keeps the first's.
func TestBinaryNodeModulesKeepToolCachesApart(t *testing.T) {
	original := t.TempDir()
	d := binary{}
	files := map[string]string{
		"vitest/package.json": "runner", ".bin/vitest": "vitest",
		".vite/vitest/results.json": "{}", ".cache/nyc/x": "cache",
	}
	for file, text := range files {
		if err := d.Write(filepath.Join(original, filepath.FromSlash(file)), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "node_modules")
	failed := func(err error) error { return fmt.Errorf("%w: %v", godog.ErrSkip, err) }
	for range 2 {
		if err := linkModules(original, dest, failed); errors.Is(err, godog.ErrSkip) {
			t.Skip(err)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Lstat(dest); err != nil || !info.IsDir() {
		t.Fatalf("node_modules is not a folder of the scenario's own: %v %v", info, err)
	}
	for _, cache := range toolCaches {
		if _, err := os.Lstat(filepath.Join(dest, cache)); !os.IsNotExist(err) {
			t.Fatalf("%s is the installation's: %v", cache, err)
		}
	}
	for _, file := range []string{"vitest/package.json", ".bin/vitest"} {
		if got, err := d.Read(filepath.Join(dest, filepath.FromSlash(file))); err != nil || string(got) != files[file] {
			t.Fatalf("linked %s: %q %v", file, got, err)
		}
	}
	if err := d.Write(filepath.Join(dest, ".vite", "vitest", "results.json"), []byte("scenario")); err != nil {
		t.Fatal(err)
	}
	if got, err := d.Read(filepath.Join(original, ".vite", "vitest", "results.json")); err != nil || string(got) != "{}" {
		t.Fatalf("the installation's cache changed: %q %v", got, err)
	}
}
