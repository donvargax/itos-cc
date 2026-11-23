package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/past"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/workdir"
)

// The in-process driver's past commits: the commits of the fixture
// repository its memory holds, read through workdir's Commits as the binary
// reads git's, and the results kept in the memory's own files, under the
// repository's .git.

// pastObjects are the objects of the fixture repository holding dir.
type pastObjects struct {
	m   *memory
	dir string
}

var _ workdir.Objects = pastObjects{}

func (o pastObjects) Top(context.Context) (string, error) {
	root, repository := o.m.repoOf(o.dir)
	if repository == nil {
		return o.dir, nil
	}
	return root, nil
}

func (o pastObjects) CommonDir(ctx context.Context) (string, error) {
	top, _ := o.Top(ctx)
	return filepath.Join(top, ".git"), nil
}

func (o pastObjects) Resolve(ctx context.Context, rev string) (string, error) {
	id, failure := o.m.history(o.dir).Resolve(ctx, rev)
	switch f := failure.(type) {
	case nil:
		return id, nil
	case mutation.NoRepository:
		return "", &past.NoRepository{Reason: f.Reason}
	case mutation.BadReference:
		return "", &past.BadRev{Rev: rev}
	case mutation.HistoryCancelled:
		return "", f.Cause
	case mutation.HistoryFailed:
		return "", f.Cause
	}
	return "", failure
}

// blobID is the id of a file's content in the fixture: its digest.
func blobID(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (o pastObjects) Entries(_ context.Context, commit string) ([]workdir.Entry, error) {
	_, repository := o.m.repoOf(o.dir)
	if repository == nil {
		return nil, errors.New("no fixture repository")
	}
	c, ok := repository.Commits[commit]
	if !ok {
		return nil, fmt.Errorf("fixture has no commit %s", commit)
	}
	var entries []workdir.Entry
	for path, text := range c.Files {
		if c.Links[path] {
			continue
		}
		entries = append(entries, workdir.Entry{Path: path, ID: blobID(text), Size: int64(len(text))})
	}
	slices.SortFunc(entries, func(a, b workdir.Entry) int { return strings.Compare(a.Path, b.Path) })
	return entries, nil
}

func (o pastObjects) Blobs(_ context.Context, ids []string) (map[string][]byte, error) {
	_, repository := o.m.repoOf(o.dir)
	if repository == nil {
		return nil, errors.New("no fixture repository")
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string][]byte{}
	for _, c := range repository.Commits {
		for _, text := range c.Files {
			if id := blobID(text); want[id] {
				out[id] = []byte(text)
			}
		}
	}
	return out, nil
}

// pastKeeper keeps files in the memory.
type pastKeeper struct{ m *memory }

var _ workdir.Keeper = pastKeeper{}

func (k pastKeeper) Read(_ context.Context, path string) ([]byte, bool) {
	text, ok := k.m.files[filepath.Clean(path)]
	return []byte(text), ok
}

func (k pastKeeper) Write(_ context.Context, path string, data []byte) error {
	path = filepath.Clean(path)
	k.m.files[path] = string(data)
	k.m.versions[path]++
	return nil
}

// past is the commits --at opens in the fixture repository holding dir.
func (m *memory) past(dir string) past.Source {
	return pastCommits(dir, pastObjects{m: m, dir: dir}, pastKeeper{m: m})
}
