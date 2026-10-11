// Package metrics writes the snapshots under .metrics/ that the tools share
// and that projects commit. Snapshots carry no timestamps, so an unchanged
// result is an unchanged file.
package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-cc/project"
)

// Name is the directory snapshots live in, at the project root.
const Name = ".metrics"

// Dir is where snapshots live: .metrics at the project root
// (project.Root), wherever a command runs, so every command reads and
// writes the same snapshots.
func Dir() string {
	return DirOf(project.Root())
}

// DirOf is where the snapshots of the project whose root is root live.
func DirOf(root string) string {
	return filepath.Join(root, Name)
}

// osRename and osReadFile are the file operations snapshots are renamed
// into place and read with; tests replace them.
var (
	osRename   = os.Rename
	osReadFile = os.ReadFile
)

// Version is the snapshot format version every snapshot records.
const Version = 1

// Write stores v as indented JSON at .metrics/name, replacing it atomically
// so a reader never sees half a snapshot. Each write stages its own file, so
// runs at the same time never rename each other's half-written one. Writes
// and reads of one snapshot in a process take turns at its rename (hold),
// and on Windows the rename retries for a moment while another process
// holds the file (retry).
func Write(name string, v any) error {
	path := filepath.Join(Dir(), name)
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
	l := hold(path)
	l.Lock()
	defer l.Unlock()
	return rename(tmp.Name(), path)
}

// Remove deletes .metrics/name; one that is not there is no error.
func Remove(name string) error {
	if err := os.Remove(filepath.Join(Dir(), name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Read loads .metrics/name into v and reports whether the file existed.
func Read(name string, v any) (bool, error) {
	return ReadIn(Dir(), name, v)
}

// ReadIn is Read of the snapshots in dir, as DirOf names it. It reads
// between renames of the snapshot in this process, and on Windows retries
// for a moment while another process renames it.
func ReadIn(dir, name string, v any) (bool, error) {
	path := filepath.Join(dir, name)
	l := hold(path)
	l.RLock()
	var data []byte
	err := retry(func() error {
		var err error
		data, err = osReadFile(path)
		return err
	})
	l.RUnlock()
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}
