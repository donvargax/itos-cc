// Package porttest holds the fakes of the past package's ports, for the
// tests of the slices that measure a past commit.
package porttest

import (
	"context"
	"sync"

	"github.com/donvargax/itos-cc/internal/past"
)

// Source opens the commits it was given, by the revisions that name them:
// each is the commit it holds with the id it is named by, all sharing its
// kept results. A revision it was not given is a past.BadRev, and Opened
// counts what it opened.
type Source struct {
	// Commits are the commits by revision, each ID set by Open from the
	// revision's value in Revs.
	Commits map[string]past.Commit
	// Results are the results the commits keep, one memory for all.
	Results *Results
	Opened  int
}

var _ past.Source = (*Source)(nil)

// Open is the commit rev names, its kept results Source's own.
func (s *Source) Open(_ context.Context, rev string) (past.Commit, error) {
	c, ok := s.Commits[rev]
	if !ok {
		return past.Commit{}, &past.BadRev{Rev: rev}
	}
	s.Opened++
	c.Results = s.Results
	return c, nil
}

// Results are kept results held in memory, by commit, command and key.
type Results struct {
	mu   sync.Mutex
	Held map[string][]byte
	// Gets and Puts count what was asked and kept.
	Gets, Puts int
}

var _ past.Results = (*Results)(nil)

func name(commit, command, key string) string { return commit + "/" + command + "-" + key }

// Get is what Put kept, or what a test set in Held.
func (r *Results) Get(_ context.Context, commit, command, key string) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Gets++
	data, ok := r.Held[name(commit, command, key)]
	return data, ok
}

// Put keeps data, replacing what was kept.
func (r *Results) Put(_ context.Context, commit, command, key string, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Puts++
	if r.Held == nil {
		r.Held = map[string][]byte{}
	}
	r.Held[name(commit, command, key)] = data
}
