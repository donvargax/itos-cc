package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	for _, sub := range []string{"", ".bin"} {
		entries, err := os.ReadDir(filepath.Join(modules, sub))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dest, sub), 0o755); err != nil { //nolint:gosec // a copied fixture's folders have the 0755 a checkout's folders have; the product under test sees fixture modes, so narrowing them would change what scenarios check
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			if slices.Contains([]string{"c8", "c8.cmd", "c8.ps1"}, name) || sub == "" && name == ".bin" {
				continue
			}
			if err := os.Symlink(filepath.Join(modules, sub, name), filepath.Join(dest, sub, name)); err != nil {
				return coverageLinkFailure(platform, required, err)
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
