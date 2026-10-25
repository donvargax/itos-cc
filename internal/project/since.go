package project

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
)

// ErrBadRef is returned by ChangedSince for a ref git cannot resolve to a
// commit.
var ErrBadRef = errors.New("not a commit in this repository")

// NoGitError is ChangedSince's error when the working directory is not in a
// git repository, or git cannot run. Reason is git's own message.
type NoGitError struct{ Reason string }

func (e *NoGitError) Error() string { return "not a git repository: " + e.Reason }

// Head returns the id of the HEAD commit of the working directory's
// repository, or a *NoGitError when it is in none or the repository has no
// commit yet.
func Head() (string, error) {
	out, err := gitCommand("rev-parse", "--verify", "HEAD^{commit}").Output()
	if err != nil {
		return "", &NoGitError{gitError(nil, err)}
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedSince returns what judge makes of each file the commits since ref
// changed: git diff ref...HEAD, so a branch's own commits whatever the base
// did since, and never the working tree. Each supported file under the
// working directory that the range changed or renamed is a key, by absolute
// path, even when none of its lines changed; its value is judge's, given the
// file as HEAD holds it, so an uncommitted edit neither adds a line nor moves
// one, the file as the range's start, the merge base, held it, under the name
// it had there, nil when it had none or one of another language, and the
// lines the range changed in HEAD's version. A deleted line counts as a
// change of the lines either side of it, marked Deletion, and a move is no
// change, so a file renamed unchanged has no lines. Both files are open only
// while judge runs. renamed maps each file the range renamed, by its absolute
// path, to the absolute path it had at ref. It reads under ctx: once ctx is
// done it starts no command and returns its cause.
func ChangedSince[T any](ctx context.Context, ref string, judge func(file, base *lang.File, lines []LineRange) T) (changed map[string]T, renamed map[string]string, err error) {
	return ChangedSinceAt(ctx, "", ref, judge)
}

// ChangedSinceAt is the same query under an explicit caller directory.
func ChangedSinceAt[T any](ctx context.Context, dir, ref string, judge func(file, base *lang.File, lines []LineRange) T) (changed map[string]T, renamed map[string]string, err error) {
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if out, err := historyCommand(ctx, dir, "rev-parse", "--git-dir").CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, nil, context.Cause(ctx)
		}
		return nil, nil, &NoGitError{gitError(out, err)}
	}
	// A ref is never read as an option of the git commands below.
	if strings.HasPrefix(ref, "-") || historyCommand(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run() != nil {
		if ctx.Err() != nil {
			return nil, nil, context.Cause(ctx)
		}
		return nil, nil, ErrBadRef
	}
	// The range starts from the merge base, as ref...HEAD diffs, whose
	// version of each file tells a function whose comments alone changed.
	out, err := historyCommand(ctx, dir, "merge-base", ref, "HEAD").Output()
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("git merge-base %s HEAD: %s", ref, gitError(nil, err))
	}
	start := strings.TrimSpace(string(out))
	// -M finds renames whatever diff.renames says.
	out, err = historyCommand(ctx, dir, "-c", "core.quotePath=false", "diff", "-U0", "-M", "--no-color", "--no-ext-diff",
		"--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--relative", "--diff-filter=AMR", ref+"...HEAD").Output()
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("git diff %s...HEAD: %s", ref, gitError(nil, err))
	}
	files, renames := diffLines(string(out))
	changed, renamed = map[string]T{}, map[string]string{}
	for name, lines := range files {
		spec := lang.Detect(name)
		if spec == nil {
			continue
		}
		abs, err := historyAbs(dir, filepath.FromSlash(name))
		if err != nil {
			return nil, nil, err
		}
		if old, ok := renames[name]; ok {
			if renamed[abs], err = historyAbs(dir, filepath.FromSlash(old)); err != nil {
				return nil, nil, err
			}
		}
		// ./ makes the path relative to the working directory, as --relative
		// made name.
		if ctx.Err() != nil {
			return nil, nil, context.Cause(ctx)
		}
		src, err := historyCommand(ctx, dir, "cat-file", "blob", "HEAD:./"+name).Output()
		if ctx.Err() != nil {
			return nil, nil, context.Cause(ctx)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("git cat-file HEAD:./%s: %s", name, gitError(nil, err))
		}
		f, err := lang.Parse(ctx, historySurroundings{Dir: dir}, spec, abs, src)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, context.Cause(ctx)
			}
			return nil, nil, err
		}
		ranges := make([]LineRange, 0, len(lines))
		for _, r := range lines {
			ranges = append(ranges, LineRange{Start: r.start, End: r.end, Deletion: r.deletion})
		}
		old := name
		if before, ok := renames[name]; ok {
			old = before
		}
		base, err := baseFile(ctx, dir, start, old, spec)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		c := judge(f, base, ranges)
		f.Close()
		if base != nil {
			base.Close()
		}
		changed[abs] = c
	}
	return changed, renamed, nil
}

// baseFile is the file at name, relative to dir, as the commit start held
// it, parsed as spec, or nil when start had no such file or it is of
// another language. Once ctx is done it returns its cause.
func baseFile(ctx context.Context, dir, start, name string, spec *lang.Spec) (*lang.File, error) {
	if lang.Detect(name) != spec {
		return nil, nil
	}
	src, err := historyCommand(ctx, dir, "cat-file", "blob", start+":./"+name).Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	if err != nil {
		// A file the range added has no base version.
		return nil, nil
	}
	abs, err := historyAbs(dir, filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	f, err := lang.Parse(ctx, historySurroundings{Dir: dir}, spec, abs, src)
	if err != nil {
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, err
	}
	return f, nil
}

// DeletedSince returns each supported file the commits since ref deleted,
// by the absolute path it had, of those under the working directory, as
// ChangedSince finds the files it changed: git diff ref...HEAD, committed
// changes only. A file renamed is not deleted: ChangedSince maps it to the
// path it had. ref must be one ChangedSince accepted.
func DeletedSince(ref string) ([]string, error) {
	return deletedSinceCommand(ref)
}

// DeletedSinceAt reads the committed deletion inventory under caller ctx/dir.
func DeletedSinceAt(ctx context.Context, dir, ref string) ([]string, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	out, err := historyCommand(ctx, dir, "-c", "core.quotePath=false", "diff", "--name-only", "-z", "-M", "--no-color", "--relative", "--diff-filter=D", ref+"...HEAD").Output()
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	return deletedNames(out, err, dir, ref)
}

func deletedSinceCommand(ref string) ([]string, error) {
	out, err := gitCommand("-c", "core.quotePath=false", "diff", "--name-only", "-z", "-M", "--no-color",
		"--relative", "--diff-filter=D", ref+"...HEAD").Output()
	return deletedNames(out, err, "", ref)
}

func deletedNames(out []byte, err error, dir, ref string) ([]string, error) {
	if err != nil {
		return nil, fmt.Errorf("git diff %s...HEAD: %s", ref, gitError(nil, err))
	}
	var deleted []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name == "" || lang.Detect(name) == nil {
			continue
		}
		abs, err := historyAbs(dir, filepath.FromSlash(name))
		if err != nil {
			return nil, err
		}
		deleted = append(deleted, abs)
	}
	return deleted, nil
}

// LineRange is a range of the lines of HEAD's version of a file that the
// commits since a ref changed, both ends included; Deletion marks the two
// lines either side of a hunk that only deleted lines.
type LineRange struct {
	Start, End int
	Deletion   bool
}

// lines is a range of line numbers, both ends included; deletion marks
// the two lines either side of a hunk that only deleted lines.
type lines struct {
	start, end int
	deletion   bool
}

// changedLines is the ranges of ranges that lines were added or changed
// in, leaving out those either side of a pure deletion.
func changedLines(ranges []lines) []LineRange {
	var out []LineRange
	for _, r := range ranges {
		if !r.deletion {
			out = append(out, LineRange{Start: r.start, End: r.end})
		}
	}
	return out
}

// diffLines reads a git diff -U0 with the prefixes a/ and b/: the lines of
// each file's new side that its hunks changed, by the file's name as git
// prints it, unquoted, and the old name of each renamed file by its new
// one. A file renamed without a change has no hunk, and no lines.
func diffLines(diff string) (map[string][]lines, map[string]string) {
	out, renames := map[string][]lines{}, map[string]string{}
	file, from, header := "", "", false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file, from, header = "", "", true
		case header && strings.HasPrefix(line, "rename from "):
			from = quotedName(strings.TrimPrefix(line, "rename from "))
		case header && strings.HasPrefix(line, "rename to "):
			if to := quotedName(strings.TrimPrefix(line, "rename to ")); to != "" && from != "" {
				renames[to] = from
				// A file renamed without a change has an entry, and no lines.
				if _, ok := out[to]; !ok {
					out[to] = nil
				}
			}
		case header && strings.HasPrefix(line, "+++ "):
			// Only in a file's header: an added line "++ x" reads "+++ x".
			file = diffName(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "@@ ") && file != "":
			header = false
			if r, ok := hunkLines(line); ok {
				out[file] = append(out[file], r)
			}
		}
	}
	return out, renames
}

// quotedName is a name in a "rename from" or "rename to" line: as is, or
// in C quotes when it holds a quote, a backslash, or a control character.
func quotedName(s string) string {
	if strings.HasPrefix(s, `"`) {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return ""
		}
		return unquoted
	}
	return s
}

// diffName is the name in a "+++ " line: "b/name", followed by a tab when
// the name holds a space, or in C quotes when it holds a quote, a
// backslash, or a control character. /dev/null, a deleted file, is "".
func diffName(s string) string {
	s = strings.TrimSuffix(s, "\t")
	if strings.HasPrefix(s, `"`) {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return ""
		}
		s = unquoted
	}
	name, ok := strings.CutPrefix(s, "b/")
	if !ok {
		return ""
	}
	return name
}

// hunkLines reads the new side of "@@ -a,b +c,d @@": lines c to c+d-1, or
// for a deletion (d = 0) the lines c and c+1 either side of it.
func hunkLines(header string) (lines, bool) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
		return lines{}, false
	}
	startText, countText, hasCount := strings.Cut(fields[2][1:], ",")
	start, err := strconv.Atoi(startText)
	if err != nil {
		return lines{}, false
	}
	count := 1
	if hasCount {
		if count, err = strconv.Atoi(countText); err != nil {
			return lines{}, false
		}
	}
	if count == 0 {
		return lines{start, start + 1, true}, true
	}
	return lines{start, start + count - 1, false}, true
}

// gitError is git's own message when it printed one, else err's.
func gitError(out []byte, err error) string {
	if ee, ok := err.(*exec.ExitError); ok && len(out) == 0 {
		out = ee.Stderr
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return msg
	}
	return err.Error()
}
