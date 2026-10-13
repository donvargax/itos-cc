// Package metrics writes the snapshots under .metrics/ that the tools share
// and that projects commit. Snapshots carry no timestamps, so an unchanged
// result is an unchanged file.
package metrics

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/snapshot"
	"github.com/donvargax/itos-cc/project"
)

// Name is the directory snapshots live in, at the project root, the
// snapshot domain's.
const Name = snapshot.Folder

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

// Version is the snapshot format version every snapshot records, the
// snapshot domain's.
const Version = snapshot.Version

// Write stores v as indented JSON at .metrics/name, replacing it atomically
// so a reader never sees half a snapshot. Each write stages its own file, so
// runs at the same time never rename each other's half-written one. Writes
// and reads of one snapshot in a process take turns at its rename (hold),
// and on Windows the rename retries for a moment while another process
// holds the file (retry).
func Write(name string, v any) error {
	return WriteIn(Dir(), name, v)
}

// WriteIn is Write of the snapshots in dir, as DirOf names it.
func WriteIn(dir, name string, v any) error {
	path := filepath.Join(dir, name)
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

// RemoveIn deletes the snapshot name in dir, as DirOf names it, then each
// folder that deleting it left empty, up to the folder at the top of name,
// such as mutate, which stays, as dir does. One that is not there is no
// error; a folder that cannot be deleted, or holds anything else, is left
// as it is.
func RemoveIn(dir, name string) error {
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	rel, err := filepath.Rel(dir, filepath.Dir(path))
	if err != nil || !filepath.IsLocal(rel) {
		return nil
	}
	for ; strings.ContainsRune(filepath.ToSlash(rel), '/'); rel = filepath.Dir(rel) {
		if os.Remove(filepath.Join(dir, rel)) != nil {
			break
		}
	}
	return nil
}

// ListIn is the names of the snapshots under the folder sub of dir, as
// DirOf names it, at any depth: each regular file whose name ends in
// .json, named from dir, slash-separated, sorted. A staging file of a
// write, a folder, a link or anything else is none, and a folder that is
// not there holds none.
func ListIn(dir, sub string) ([]string, error) {
	top := filepath.Join(dir, sub)
	if _, err := os.Lstat(top); os.IsNotExist(err) {
		return nil, nil
	}
	var names []string
	err := filepath.WalkDir(top, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	return names, nil
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
