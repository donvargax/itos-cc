package testinputs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/project"
)

// goEnvKeys are the build environment strict Go evidence records, as go env
// resolves them in the module, so a value given, left to its default or set
// in go env's own file reads the same. GOTOOLCHAIN only says how the version
// was chosen, so it is left out; GOVERSION is the project toolchain's.
var goEnvKeys = []string{
	"GOVERSION", "GOHOSTOS", "GOHOSTARCH", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS",
	"GOEXPERIMENT", "GOAMD64", "GOARM", "GO386",
}

// GoCoverageInputs fingerprints the nearest module's input boundary for
// independently measured executable coverage: its go.mod and go.sum, the
// Go files a build of the checkout can include (goModuleFiles), each
// without the summary comment itos-cc writes, the project's itos-cc.yaml,
// the configured support files, the producer and the Go environment.
func GoCoverageInputs(ctx context.Context, runtime mutants.RuntimeInputs, source, projectRoot, producer string, support map[string]string) (map[string]string, error) {
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
	files, err := goModuleFiles(ctx, module)
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		if err := addFile(path); err != nil {
			return nil, err
		}
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
	env, err := goEnv(ctx, runtime, module)
	if err != nil {
		return nil, err
	}
	out["@go-env"] = hashText(env)
	return out, nil
}

// goModuleFiles are the module's go.mod and go.sum, and the Go files beneath
// it that a build of the checkout can include: those git tracks, read from
// the working tree, or, outside a git repository, those on the disk. Either
// way go's own directory rules leave out testdata, a directory or file whose
// name starts with _ or ., and a nested module (goBuilds).
func goModuleFiles(ctx context.Context, module string) ([]string, error) {
	var files []string
	for _, name := range []string{"go.mod", "go.sum"} {
		if path := filepath.Join(module, name); goCoverageFileExists(path) {
			files = append(files, path)
		}
	}
	candidates, err := project.TrackedAt(ctx, module)
	var noGit *project.NoGitError
	if errors.As(err, &noGit) {
		candidates, err = goDiskFiles(ctx, module)
	}
	if err != nil {
		return nil, err
	}
	builds := goBuilds{module: module, nested: map[string]bool{}}
	for _, rel := range candidates {
		path := filepath.Join(module, filepath.FromSlash(rel))
		// A tracked file deleted from the working tree is in no build.
		if builds.includes(rel) && goCoverageFileExists(path) {
			files = append(files, path)
		}
	}
	slices.Sort(files)
	return files, nil
}

// goDiskFiles are the slash paths, from module, of the files beneath it on
// the disk, never inside a directory go's rules skip.
func goDiskFiles(ctx context.Context, module string) ([]string, error) {
	builds := goBuilds{module: module, nested: map[string]bool{}}
	var rels []string
	err := project.WalkDir(ctx, module, func(_ context.Context, path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == module {
			return nil
		}
		rel, err := filepath.Rel(module, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if goSkipsName(entry.Name()) || builds.nestedModule(path) {
				return filepath.SkipDir
			}
			return nil
		}
		rels = append(rels, rel)
		return nil
	})
	return rels, err
}

// goBuilds says which files beneath module a build of it can include.
type goBuilds struct {
	module string
	nested map[string]bool
}

// includes is whether the file at rel, a slash path from the module, is a
// Go file go's directory rules let a build of the module include.
func (b goBuilds) includes(rel string) bool {
	parts := strings.Split(rel, "/")
	name := parts[len(parts)-1]
	if !strings.HasSuffix(name, ".go") || goSkipsName(name) {
		return false
	}
	dir := b.module
	for _, part := range parts[:len(parts)-1] {
		dir = filepath.Join(dir, part)
		if goSkipsName(part) || b.nestedModule(dir) {
			return false
		}
	}
	return true
}

// nestedModule is whether dir, beneath the module, holds a go.mod of its own.
func (b goBuilds) nestedModule(dir string) bool {
	nested, known := b.nested[dir]
	if !known {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		nested = err == nil
		b.nested[dir] = nested
	}
	return nested
}

// goSkipsName is whether go leaves out a directory or file of this name.
func goSkipsName(name string) bool {
	return name == "testdata" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// goEnv is the module's Go environment as go env resolves it there, in the
// run's environment: each of goEnvKeys with its value, NUL-separated.
func goEnv(ctx context.Context, runtime mutants.RuntimeInputs, module string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", append([]string{"env", "-json"}, goEnvKeys...)...) //nolint:gosec // only the fixed go program, asked for the fixed variables above
	cmd.Dir = module
	cmd.Env = []string{}
	for _, key := range slices.Sorted(maps.Keys(runtime.Env)) {
		cmd.Env = append(cmd.Env, key+"="+runtime.Env[key])
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	if err != nil {
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return "", fmt.Errorf("go env in %s: %s", module, text)
		}
		return "", fmt.Errorf("go env in %s: %w", module, err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return "", fmt.Errorf("go env in %s: %w", module, err)
	}
	var env []string
	for _, key := range goEnvKeys {
		env = append(env, key+"="+values[key])
	}
	return strings.Join(env, "\x00"), nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func goCoverageFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
