package porttest

import (
	"bufio"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/mutants"
)

func (s *Inputs) goUnsupported(ctx context.Context, source string) (string, mutants.InputFailure) {
	module := s.nearest(ctx, source, "go.mod")
	if module == "" {
		return "", inputError(fmt.Errorf("%s: no nearest go.mod", filepath.Dir(source)))
	}
	setting := s.Runtime.Env["GOWORK"]
	if setting != "off" {
		if setting != "" && setting != "auto" {
			path := setting
			if !filepath.IsAbs(path) {
				path = filepath.Join(module, path)
			}
			if s.regular(ctx, path) {
				return filepath.Clean(path), nil
			}
		} else {
			for dir := module; ; dir = filepath.Dir(dir) {
				path := filepath.Join(dir, "go.work")
				if s.regular(ctx, path) {
					return path, nil
				}
				if filepath.Dir(dir) == dir {
					break
				}
			}
		}
	}
	data, failure := s.required(ctx, filepath.Join(module, "go.mod"))
	if failure != nil {
		return "", failure
	}
	for _, target := range localGoReplacements([]byte(data)) {
		path := target
		if !filepath.IsAbs(path) {
			path = filepath.Join(module, path)
		}
		path = filepath.Clean(path)
		if !inside(module, path) {
			return target, nil
		}
		rel, _ := filepath.Rel(module, path)
		if rel == "." {
			continue
		}
		current := module
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			if _, linked := s.Links[current]; linked {
				return target, nil
			}
			if s.Exists(ctx, filepath.Join(current, "go.mod")) {
				return target, nil
			}
		}
	}
	return "", inputStopped(ctx)
}

func localGoReplacements(data []byte) []string {
	var targets []string
	inside := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if cut := strings.Index(line, "//"); cut >= 0 {
			line = strings.TrimSpace(line[:cut])
		}
		if line == "" {
			continue
		}
		if line == "replace (" {
			inside = true
			continue
		}
		if inside && line == ")" {
			inside = false
			continue
		}
		if !inside {
			if !strings.HasPrefix(line, "replace ") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "replace "))
		}
		fields := goModFields(line)
		arrow := -1
		for i, field := range fields {
			if field == "=>" {
				arrow = i
				break
			}
		}
		if arrow < 0 || arrow+1 >= len(fields) {
			continue
		}
		// A version after the replacement module path is a module-version
		// replacement, not a local directory.
		if arrow+2 < len(fields) && fields[arrow+2] != "" {
			continue
		}
		path := fields[arrow+1]
		if filepath.IsAbs(path) || strings.HasPrefix(path, ".") {
			targets = append(targets, path)
		}
	}
	return targets
}

func goModFields(line string) []string {
	var fields []string
	for len(line) > 0 {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if line[0] == '"' || line[0] == '`' {
			quote := line[0]
			end := 1
			for end < len(line) {
				if line[end] == quote && (quote == '`' || line[end-1] != '\\') {
					end++
					break
				}
				end++
			}
			if end > len(line) || line[end-1] != quote {
				break
			}
			value, err := strconv.Unquote(line[:end])
			if err == nil {
				fields = append(fields, value)
			}
			line = line[end:]
			continue
		}
		end := strings.IndexAny(line, " \t")
		if end < 0 {
			fields = append(fields, line)
			break
		}
		fields = append(fields, line[:end])
		line = line[end:]
	}
	return fields
}
