package workdir

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/past"
	"github.com/donvargax/itos-cc/internal/project"
)

// Git is the objects of the git repository holding Dir, read through the git
// command: ls-tree for a commit's files, cat-file --batch for their
// contents, so a commit is read without a checkout and the disk's files
// never.
type Git struct{ Dir string }

var _ Objects = Git{}

// Top is the project root of Dir.
func (g Git) Top(context.Context) (string, error) { return project.RootOf(g.Dir), nil }

// CommonDir is the git common directory of Dir's repository, absolute: the
// same for the main worktree and every linked one.
func (g Git) CommonDir(ctx context.Context) (string, error) {
	out, err := project.GitIn(ctx, g.Dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", g.failure(ctx, err, "")
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(g.Dir, dir)
	}
	return filepath.Clean(dir), nil
}

// Resolve is the id of the commit rev names.
func (g Git) Resolve(ctx context.Context, rev string) (string, error) {
	id, err := project.ResolveAt(ctx, g.Dir, rev)
	if err == nil {
		return id, nil
	}
	if ctx.Err() != nil {
		return "", context.Cause(ctx)
	}
	var noGit *project.NoGitError
	switch {
	case errors.As(err, &noGit):
		return "", &past.NoRepository{Reason: noGit.Reason}
	case errors.Is(err, project.ErrBadRef):
		return "", &past.BadRev{Rev: rev}
	}
	return "", err
}

// failure is err, of a git command that failed, as the failure it is: a
// cancellation, else no repository to ask.
func (g Git) failure(ctx context.Context, err error, _ string) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return &past.NoRepository{Reason: err.Error()}
}

// Entries are the regular files of the commit's tree, as git ls-tree lists
// them whole: executable or not, never a link, a submodule or a folder.
func (g Git) Entries(ctx context.Context, commit string) ([]Entry, error) {
	var stderr bytes.Buffer
	cmd := project.GitIn(ctx, g.Dir, "ls-tree", "-r", "-z", "-l", "--full-tree", commit)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, fmt.Errorf("git ls-tree %s: %s", commit, strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	var entries []Entry
	for _, record := range strings.Split(string(out), "\x00") {
		meta, name, ok := strings.Cut(record, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 4 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			continue
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git ls-tree %s: a size %q is no number", commit, fields[3])
		}
		entries = append(entries, Entry{Path: name, ID: fields[2], Size: size})
	}
	return entries, nil
}

// Blobs are the contents of the blobs ids name, read in one run of git
// cat-file --batch.
func (g Git) Blobs(ctx context.Context, ids []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cmd := project.GitIn(ctx, g.Dir, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(ids, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(pipe)
	var readErr error
	for _, id := range ids {
		if readErr = readBlob(reader, id, out); readErr != nil {
			break
		}
	}
	if readErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if readErr != nil {
		return nil, fmt.Errorf("git cat-file: %v %s", readErr, strings.TrimSpace(stderr.String()))
	}
	if waitErr != nil {
		return nil, fmt.Errorf("git cat-file: %v %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// readBlob reads the answer to one id of cat-file --batch: a line
// "<id> blob <size>", the content and a newline.
func readBlob(r *bufio.Reader, id string, into map[string][]byte) error {
	header, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[1] != "blob" {
		return fmt.Errorf("%s is no blob: %q", id, strings.TrimSpace(header))
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return fmt.Errorf("%s has the size %q", id, fields[2])
	}
	text := make([]byte, size+1)
	if _, err := io.ReadFull(r, text); err != nil {
		return err
	}
	into[id] = text[:size]
	return nil
}

// Disk is the Keeper of the files of this system.
type Disk struct{}

var _ Keeper = Disk{}

// Read is the file at path.
func (Disk) Read(ctx context.Context, path string) ([]byte, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	text, err := os.ReadFile(path)
	return text, err == nil
}

// Write writes data to a file of its own beside path and renames it there,
// so no reader sees part of it.
func (Disk) Write(ctx context.Context, path string, data []byte) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	staging, err := os.CreateTemp(dir, ".staging-*")
	if err != nil {
		return err
	}
	_, werr := staging.Write(data)
	cerr := staging.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(staging.Name())
		return err
	}
	if err := os.Rename(staging.Name(), path); err != nil {
		_ = os.Remove(staging.Name())
		return err
	}
	return nil
}
