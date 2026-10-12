// Package porttest provides in-memory fakes for snapshot-domain tests.
package porttest

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// Store is a project's snapshots held in memory: its root, the JSON each
// snapshot holds, and configured failures. It reads and writes as
// internal/metricsdir's Store does on the disk, which StoreContract holds.
type Store struct {
	// RootDir is what Root gives: the project root, absolute.
	RootDir string
	// Snapshots are the JSON of each snapshot, by name; Write stores there.
	Snapshots map[string]string
	// Failures are Read's and Write's, by name: a snapshot named here
	// fails with its failure.
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
	if failure, ok := s.Failures[name]; ok {
		return false, failure
	}
	text, ok := s.Snapshots[name]
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
	if failure, ok := s.Failures[name]; ok {
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
	s.Snapshots[name] = data.String()
	return nil
}
