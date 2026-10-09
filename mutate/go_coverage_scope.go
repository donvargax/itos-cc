package mutate

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GoCoverageUnsupported returns the responsible workspace or local replace
// path when source's effective Go build scope reaches beyond its inventoried
// nearest module. It never opens a workspace or an out-of-module target.
func GoCoverageUnsupported(source string) (string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	module, err := nearestGoModule(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	if workspace := activeGoWorkspace(module); workspace != "" {
		return workspace, nil
	}
	data, err := os.ReadFile(filepath.Join(module, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, target := range localGoReplacements(data) {
		path := target
		if !filepath.IsAbs(path) {
			path = filepath.Join(module, path)
		}
		path = filepath.Clean(path)
		rel, relErr := filepath.Rel(module, path)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return target, nil
		}
		if rel == "." {
			continue
		}
		// A symlink can lead outside the inventory. Inspect path components
		// with Lstat only; never resolve or traverse the target.
		current := module
		unsupported := false
		for _, component := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			info, statErr := os.Lstat(current)
			if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
				unsupported = true
				break
			}
			if _, statErr := os.Lstat(filepath.Join(current, "go.mod")); statErr == nil {
				unsupported = true
				break
			}
		}
		if unsupported {
			return target, nil
		}
	}
	return "", nil
}

func nearestGoModule(dir string) (string, error) {
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s: no nearest go.mod", dir)
		}
		dir = parent
	}
}

func activeGoWorkspace(module string) string {
	setting := os.Getenv("GOWORK")
	if setting == "off" {
		return ""
	}
	if setting != "" && setting != "auto" {
		path := setting
		if !filepath.IsAbs(path) {
			path = filepath.Join(module, path)
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return filepath.Clean(path)
		}
		return ""
	}
	for dir := module; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, "go.work")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
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
