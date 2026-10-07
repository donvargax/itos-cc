// Package metrics writes the snapshots under .metrics/ that the tools share
// and that projects commit. Snapshots carry no timestamps, so an unchanged
// result is an unchanged file.
package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// Dir is where snapshots live, relative to the working directory.
const Dir = ".metrics"

// Version is the snapshot format version every snapshot records.
const Version = 1

// Write stores v as indented JSON at .metrics/name, replacing it atomically
// so a reader never sees half a snapshot. Each write stages its own file, so
// runs at the same time never rename each other's half-written one; on
// Windows the rename retries for a moment while another one holds the file.
func Write(name string, v any) error {
	path := filepath.Join(Dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return rename(tmp.Name(), path)
}

// Remove deletes .metrics/name; one that is not there is no error.
func Remove(name string) error {
	if err := os.Remove(filepath.Join(Dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Read loads .metrics/name into v and reports whether the file existed.
func Read(name string, v any) (bool, error) {
	data, err := os.ReadFile(filepath.Join(Dir, name))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}
