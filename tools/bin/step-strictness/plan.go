// Command step-strictness checks explicitly reviewed acceptance assertion edits.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type plan struct {
	Version         int     `json:"version"`
	Root            string  `json:"root"`
	OutputDir       string  `json:"output_dir"`
	DeadlineSeconds int     `json:"deadline_seconds"`
	Probes          []probe `json:"probes"`
}
type probe struct {
	Name        string    `json:"name"`
	ScenarioID  string    `json:"scenario_id"`
	SourcePath  string    `json:"source_path"`
	Original    string    `json:"original"`
	Replacement string    `json:"replacement"`
	Assertion   assertion `json:"assertion"`
}
type assertion struct {
	URI        string `json:"uri"`
	Line       int    `json:"line"`
	Name       string `json:"name"`
	ErrorToken string `json:"error_token"`
}
type source struct {
	Path         string      `json:"path"`
	Mode         os.FileMode `json:"mode"`
	OriginalHash string      `json:"original_sha256"`
	Backup       string      `json:"backup"`
	EditedHashes []string    `json:"edited_sha256"`
	bytes        []byte
}
type prepared struct {
	plan         plan
	root, output string
	probes       []preparedProbe
	sources      map[string]*source
}
type preparedProbe struct {
	probe
	path   string
	edited []byte
}

// checkJSON checks duplicate keys independently of typed decoding, which otherwise
// accepts the last value. Paths make all duplicate/unknown keys actionable.
func checkJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var problems []error
	var read func(string) error
	read = func(path string) error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if t == json.Delim('{') {
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name := key.(string)
				if seen[name] {
					problems = append(problems, fmt.Errorf("%s.%s: duplicate key", path, name))
				}
				seen[name] = true
				if err := read(path + "." + name); err != nil {
					return err
				}
			}
			_, err = d.Token()
		} else if t == json.Delim('[') {
			for i := 0; d.More(); i++ {
				if err := read(fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			_, err = d.Token()
		}
		return err
	}
	if err := read("plan"); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("plan: expected one JSON document")
	}
	return errors.Join(problems...)
}

func loadPlan(path string) (plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return plan{}, err
	}
	if err = checkJSON(data); err != nil {
		return plan{}, err
	}
	// Refuse all unknown fields, including inside probes and assertion objects.
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil {
		return plan{}, err
	}
	var problems []error
	var nulls func(any, string)
	nulls = func(v any, path string) {
		switch value := v.(type) {
		case nil:
			problems = append(problems, fmt.Errorf("%s: null is not a value", path))
		case map[string]any:
			for k, v := range value {
				nulls(v, path+"."+k)
			}
		case []any:
			for i, v := range value {
				nulls(v, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	var values any
	if err = json.Unmarshal(data, &values); err != nil {
		return plan{}, err
	}
	nulls(values, "plan")
	unknown := func(path string, obj map[string]json.RawMessage, keys ...string) {
		for k := range obj {
			found := false
			for _, want := range keys {
				if want == k {
					found = true
				}
			}
			if !found {
				problems = append(problems, fmt.Errorf("%s.%s: unknown key", path, k))
			}
		}
	}
	unknown("plan", raw, "version", "root", "output_dir", "deadline_seconds", "probes")
	var probes []map[string]json.RawMessage
	if err = json.Unmarshal(raw["probes"], &probes); err != nil {
		return plan{}, fmt.Errorf("plan.probes: %w", err)
	}
	for i, p := range probes {
		at := fmt.Sprintf("plan.probes[%d]", i)
		unknown(at, p, "name", "scenario_id", "source_path", "original", "replacement", "assertion")
		var a map[string]json.RawMessage
		if err = json.Unmarshal(p["assertion"], &a); err != nil {
			problems = append(problems, fmt.Errorf("%s.assertion: %w", at, err))
		} else {
			unknown(at+".assertion", a, "uri", "line", "name", "error_token")
		}
	}
	if err = errors.Join(problems...); err != nil {
		return plan{}, err
	}
	p := plan{DeadlineSeconds: 600}
	if err = json.Unmarshal(data, &p); err != nil {
		return plan{}, err
	}
	return p, nil
}

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var idPattern = regexp.MustCompile(`^ID-[A-Z]+-[0-9]+$`)

func prepare(path string) (prepared, error) {
	p, err := loadPlan(path)
	if err != nil {
		return prepared{}, err
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return prepared{}, err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return prepared{}, err
	}
	q := prepared{plan: p, sources: map[string]*source{}}
	var problems []error
	bad := func(at, why string) { problems = append(problems, fmt.Errorf("%s: %s", at, why)) }
	if p.Version != 1 {
		bad("version", "only version 1 is supported")
	}
	if p.Root == "" || filepath.IsAbs(p.Root) || strings.Contains(p.Root, `\`) {
		bad("root", "required slash path relative to the manifest")
	}
	q.root, err = filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(p.Root)))
	if err != nil {
		return q, err
	}
	if p.DeadlineSeconds < 1 || p.DeadlineSeconds > 600 {
		bad("deadline_seconds", "must be between 1 and 600; go test's timeout stays unchanged")
	}
	if len(p.Probes) == 0 {
		bad("probes", "at least one probe is required")
	}
	if p.OutputDir != "" {
		if filepath.IsAbs(p.OutputDir) || strings.Contains(p.OutputDir, `\`) {
			bad("output_dir", "use a slash path relative to the manifest")
		}
		q.output = filepath.Clean(filepath.Join(base, filepath.FromSlash(p.OutputDir)))
		if strings.Contains(q.output, ",") {
			bad("output_dir", "commas are Godog formatter separators, not report path characters")
		}
		if within(q.root, q.output) {
			bad("output_dir", "recovery and logs must be outside the checkout")
		}
		if err = noLinks(q.output, true); err != nil {
			bad("output_dir", err.Error())
		}
		if _, err = os.Lstat(q.output); !os.IsNotExist(err) {
			bad("output_dir", "must not already exist")
		}
	}
	seen := map[string]bool{}
	for i, p := range p.Probes {
		at := fmt.Sprintf("probes[%d]", i)
		if !labelPattern.MatchString(p.Name) || seen[p.Name] {
			bad(at+".name", "required unique label of letters, digits, _ or -")
		}
		seen[p.Name] = true
		if !idPattern.MatchString(p.ScenarioID) {
			bad(at+".scenario_id", "expected one ID-AREA-number, without @")
		}
		if p.Assertion.URI == "" || p.Assertion.Line < 1 || p.Assertion.Name == "" || strings.TrimSpace(p.Assertion.ErrorToken) == "" {
			bad(at+".assertion", "uri, positive line, name and nonempty error_token are required")
		}
		if p.Original == "" || p.Replacement == "" || p.Original == p.Replacement {
			bad(at, "original and replacement must be nonempty and different")
		}
		file := filepath.Clean(filepath.Join(base, filepath.FromSlash(p.SourcePath)))
		rel, _ := filepath.Rel(q.root, file)
		if p.SourcePath == "" || filepath.IsAbs(p.SourcePath) || strings.Contains(p.SourcePath, `\`) || !strings.HasPrefix(filepath.ToSlash(rel), "features/") || (filepath.Ext(file) != ".go" && filepath.Ext(file) != ".feature") {
			bad(at+".source_path", "must name acceptance .go or .feature source under features, relative to the manifest")
			continue
		}
		if err = noLinks(file, false); err != nil {
			bad(at+".source_path", err.Error())
			continue
		}
		s, ok := q.sources[file]
		if !ok {
			info, err := os.Stat(file)
			if err != nil {
				bad(at+".source_path", err.Error())
				continue
			}
			if !info.Mode().IsRegular() {
				bad(at+".source_path", "not a regular file")
				continue
			}
			if err = sourceMode(info.Mode()); err != nil {
				bad(at+".source_path", err.Error())
				continue
			}
			b, err := os.ReadFile(file)
			if err != nil {
				bad(at+".source_path", err.Error())
				continue
			}
			s = &source{Path: file, Mode: info.Mode().Perm(), OriginalHash: hash(b), bytes: b}
			q.sources[file] = s
		}
		if strings.Count(string(s.bytes), p.Original) != 1 {
			bad(at+".original", "must match exactly once; include unique surrounding text")
			continue
		}
		edited := bytes.Replace(s.bytes, []byte(p.Original), []byte(p.Replacement), 1)
		s.EditedHashes = append(s.EditedHashes, hash(edited))
		q.probes = append(q.probes, preparedProbe{probe: p, path: file, edited: edited})
	}
	return q, errors.Join(problems...)
}

func sourceMode(mode os.FileMode) error {
	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("special permission bits are not supported; source is unchanged")
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// noLinks includes every existing component, not just the edited leaf. A new
// output directory may be missing, but its parent must already exist.
func noLinks(path string, newLeaf bool) error {
	if newLeaf {
		path = filepath.Dir(path)
	}
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinked path %s", path)
		}
		next := filepath.Dir(path)
		if next == path {
			return nil
		}
		path = next
	}
}
