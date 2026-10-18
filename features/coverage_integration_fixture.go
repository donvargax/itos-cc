package features

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/cucumber/godog"
)

// Validate refuses contradictory fixture declarations before tool probing.
func (c Coverage) Validate() error {
	if c.WithoutC8 && c.Language != "typescript" {
		return fmt.Errorf("WithoutC8 requires TypeScript, got %s", c.Language)
	}
	if c.WithoutC8 && strings.TrimSpace(c.IntegrationReport) != "" {
		return fmt.Errorf("WithoutC8 cannot declare an integration report")
	}
	return nil
}

// coverageWithoutC8 creates only owned links; it never edits the original
// installation. Refusing an existing destination prevents following a fixture's
// previous full-installation link and accidentally editing the pinned packages.
func coverageWithoutC8(modules, dest, platform string, required bool) error {
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("missing-c8 facade destination already exists: %s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}
	return linkPackages(modules, dest, []string{"c8", "c8.cmd", "c8.ps1"}, func(err error) error {
		return coverageLinkFailure(platform, required, err)
	})
}

// toolCaches are the folders of a node_modules its tools write as they run:
// Vite's, so Vitest's results and dependencies (.vite), and the one Babel,
// nyc and others share (.cache). Every scenario's node_modules links them
// to a folder of its own outside the scenario's tree (scenarioCaches),
// never the pinned installation's, which every scenario links and several
// would write at once when scenarios run at once (T-128). Outside the
// tree, as the installation's were when node_modules linked it whole, so
// what a scenario sees of its tree, such as a counted run's leaving the
// live tree unchanged, is what it saw before.
var toolCaches = []string{".vite", ".cache"}

var (
	toolCachesMu  sync.Mutex
	toolCachesDir string
)

// scenarioCaches makes a folder for one scenario's tool caches, in the
// system's temporary directory, inside a folder of the run's that
// removeToolCaches removes.
func scenarioCaches() (string, error) {
	toolCachesMu.Lock()
	defer toolCachesMu.Unlock()
	if toolCachesDir == "" {
		dir, err := os.MkdirTemp("", "itos-cc-features-caches-")
		if err != nil {
			return "", err
		}
		toolCachesDir = dir
	}
	return os.MkdirTemp(toolCachesDir, "scenario-")
}

// removeToolCaches removes the folders scenarioCaches made.
func removeToolCaches() {
	toolCachesMu.Lock()
	defer toolCachesMu.Unlock()
	if toolCachesDir != "" {
		_ = os.RemoveAll(toolCachesDir)
		toolCachesDir = ""
	}
}

// linkPackages makes dest a node_modules of links to each entry of modules
// and of its .bin, leaving out the names in leftOut, and toolCaches linked
// to the scenario's own caches folder instead, with failed links' errors
// as failedLink makes them. It never writes in modules.
func linkPackages(modules, dest string, leftOut []string, failedLink func(error) error) error {
	leftOut = slices.Concat(toolCaches, leftOut)
	caches, err := scenarioCaches()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil { //nolint:gosec // a copied fixture's folders have the 0755 a checkout's folders have; the product under test sees fixture modes, so narrowing them would change what scenarios check
		return err
	}
	for _, cache := range toolCaches {
		own := filepath.Join(caches, strings.TrimPrefix(cache, "."))
		if err := os.Mkdir(own, 0o755); err != nil { //nolint:gosec // a tool cache's folder has the 0755 the tool would give it
			return err
		}
		if err := os.Symlink(own, filepath.Join(dest, cache)); err != nil {
			return failedLink(err)
		}
	}
	for _, sub := range []string{"", ".bin"} {
		entries, err := os.ReadDir(filepath.Join(modules, sub))
		if errors.Is(err, fs.ErrNotExist) && sub != "" {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dest, sub), 0o755); err != nil { //nolint:gosec // a copied fixture's folders have the 0755 a checkout's folders have; the product under test sees fixture modes, so narrowing them would change what scenarios check
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			if slices.Contains(leftOut, name) || sub == "" && name == ".bin" {
				continue
			}
			if err := os.Symlink(filepath.Join(modules, sub, name), filepath.Join(dest, sub, name)); err != nil {
				return failedLink(err)
			}
		}
	}
	return nil
}

// coverageLinkFailure admits only typed capability failures as omissions.
// ERROR_PRIVILEGE_NOT_HELD is specific to Windows and not named by syscall.
func coverageLinkFailure(platform string, required bool, err error) error {
	const privilegeNotHeld syscall.Errno = 1314
	unsupported := errors.Is(err, errors.ErrUnsupported) || errors.Is(err, os.ErrPermission)
	if platform == "windows" && errors.Is(err, privilegeNotHeld) {
		unsupported = true
	}
	if required || !unsupported {
		return err
	}
	return fmt.Errorf("%w: symlinks unavailable: %w", godog.ErrSkip, err)
}
