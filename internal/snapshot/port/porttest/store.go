// Package porttest provides in-memory fakes for snapshot-domain tests.
package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Store is a project's snapshots held in memory: its root, the JSON each
// snapshot holds, and configured failures. It reads, writes, removes and
// lists as internal/metricsdir's Store does on the disk, which
// StoreContract holds. It holds no folders, so a removal leaves none
// empty. Every name is taken slash-separated, as the disk takes a
// snapshot's name in either form where a backslash separates.
type Store struct {
	// RootDir is what Root gives: the project root, absolute.
	RootDir string
	// Snapshots are the JSON of each snapshot, by its name,
	// slash-separated; Write stores there, and Remove deletes there.
	Snapshots map[string]string
	// Failures are Read's, Write's and Remove's, by the snapshot's name,
	// and List's, by the folder's: a name here fails with its failure.
	Failures map[string]port.Failure
	// RootFailure, when set, is Root's failure.
	RootFailure port.Failure
}

var _ port.Store = (*Store)(nil)

// Root is RootDir, RootFailure when it is set, or Cancelled once ctx is
// done.
func (s *Store) Root(ctx context.Context) (string, port.Failure) {
	if ctx.Err() != nil {
		return "", &port.Cancelled{}
	}
	if s.RootFailure != nil {
		return "", s.RootFailure
	}
	return s.RootDir, nil
}

// Read decodes the snapshot's JSON into v: one not in Snapshots is false,
// one that does not decode Unreadable, with the decoder's text.
func (s *Store) Read(ctx context.Context, name string, v any) (bool, port.Failure) {
	if ctx.Err() != nil {
		return false, &port.Cancelled{}
	}
	if failure, ok := s.Failures[filepath.ToSlash(name)]; ok {
		return false, failure
	}
	text, ok := s.Snapshots[filepath.ToSlash(name)]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(text), v); err != nil {
		return true, &port.Unreadable{Name: name, Reason: err.Error()}
	}
	return true, nil
}

// Write stores v's JSON as the snapshot name, in the metrics library's
// bytes: indented by two spaces, nothing escaped for HTML, ending in a
// newline. One that does not encode is Unwritable, with the encoder's
// text, and stores nothing.
func (s *Store) Write(ctx context.Context, name string, v any) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{}
	}
	if failure, ok := s.Failures[filepath.ToSlash(name)]; ok {
		return failure
	}
	var data bytes.Buffer
	enc := json.NewEncoder(&data)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return &port.Unwritable{Name: name, Reason: err.Error()}
	}
	if s.Snapshots == nil {
		s.Snapshots = map[string]string{}
	}
	s.Snapshots[filepath.ToSlash(name)] = data.String()
	return nil
}

// Remove deletes the snapshot name from Snapshots; one not there is no
// failure.
func (s *Store) Remove(ctx context.Context, name string) port.Failure {
	if ctx.Err() != nil {
		return &port.Cancelled{}
	}
	if failure, ok := s.Failures[filepath.ToSlash(name)]; ok {
		return failure
	}
	delete(s.Snapshots, filepath.ToSlash(name))
	return nil
}

// List is the names in Snapshots under the folder dir that end in .json,
// sorted.
func (s *Store) List(ctx context.Context, dir string) ([]string, port.Failure) {
	if ctx.Err() != nil {
		return nil, &port.Cancelled{}
	}
	if failure, ok := s.Failures[filepath.ToSlash(dir)]; ok {
		return nil, failure
	}
	prefix := path.Clean(filepath.ToSlash(dir)) + "/"
	var names []string
	for name := range s.Snapshots {
		if (prefix == "./" || strings.HasPrefix(name, prefix)) && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}
