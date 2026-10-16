package testinputs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// GoCoverageInputs fingerprints the conservative nearest-module input
// boundary for independently measured executable coverage.
func GoCoverageInputs(runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	module := filepath.Dir(abs)
	for {
		if info, statErr := os.Stat(filepath.Join(module, "go.mod")); statErr == nil && !info.IsDir() {
			break
		}
		parent := filepath.Dir(module)
		if parent == module {
			return nil, fmt.Errorf("%s: no nearest go.mod", source)
		}
		module = parent
	}
	out := map[string]string{}
	addFile := func(path string) error {
		data, readErr := os.ReadFile(path) //nolint:gosec // fingerprint actual files inside the selected module inventory
		if readErr != nil {
			return readErr
		}
		if strings.HasSuffix(path, ".go") {
			if spec := lang.Detect(path); spec != nil && spec.Name == "go" {
				data = []byte(mutants.StripAnnotation(string(data), spec.Comment))
			}
		}
		rel, relErr := filepath.Rel(projectRoot, path)
		if relErr != nil {
			return relErr
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	}
	err = filepath.WalkDir(module, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != module && entry.IsDir() {
			if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" {
			return addFile(path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if config := filepath.Join(projectRoot, "itos-cc.yaml"); goCoverageFileExists(config) {
		if err := addFile(config); err != nil {
			return nil, err
		}
	} else {
		out["itos-cc.yaml"] = "absent"
	}
	for path, hash := range support {
		out["support:"+path] = hash
	}
	out["@producer"] = hashText(producer)
	out["@go-env"] = hashText(strings.Join([]string{
		runtime.GoVersion, runtime.Platform, runtime.Architecture,
		"GOOS=" + runtime.Env["GOOS"], "GOARCH=" + runtime.Env["GOARCH"],
		"CGO_ENABLED=" + runtime.Env["CGO_ENABLED"], "GOFLAGS=" + runtime.Env["GOFLAGS"],
		"GOEXPERIMENT=" + runtime.Env["GOEXPERIMENT"], "GOTOOLCHAIN=" + runtime.Env["GOTOOLCHAIN"],
		"GOAMD64=" + runtime.Env["GOAMD64"], "GOARM=" + runtime.Env["GOARM"],
		"GO386=" + runtime.Env["GO386"],
	}, "\x00"))
	return out, nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func goCoverageFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
