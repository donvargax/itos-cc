package project

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/donvargax/itos-cc/lang"
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
	out, err := exec.Command("git", "rev-parse", "--verify", "HEAD^{commit}").Output()
	if err != nil {
		return "", &NoGitError{gitError(nil, err)}
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedSince returns the functions the commits since ref changed, by
// file: git diff ref...HEAD, so a branch's own commits whatever the base did
// since, and never the working tree. Each supported file under the working
// directory that the range changed or renamed is a key, by absolute path,
// even when no function of it changed; its value holds the namespace#name of
// each function whose lines the range touched. A deleted line counts as a
// change of the lines either side of it, and a move is no change, so a file
// renamed unchanged has none. Functions are found in HEAD's version of the
// file, so an uncommitted edit neither adds one nor moves its lines. renamed
// maps each file the range renamed, by its absolute path, to the absolute
// path it had at ref. hash, when set, fingerprints a function's text, so
// functions sharing a name are told apart (see ChangedFunctions).
func ChangedSince(ref string, hash func(*lang.File, lang.Unit) string) (changed map[string]ChangedFunctions, renamed map[string]string, err error) {
	if out, err := exec.Command("git", "rev-parse", "--git-dir").CombinedOutput(); err != nil {
		return nil, nil, &NoGitError{gitError(out, err)}
	}
	// A ref is never read as an option of the git commands below.
	if strings.HasPrefix(ref, "-") || exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run() != nil {
		return nil, nil, ErrBadRef
	}
	// -M finds renames whatever diff.renames says.
	out, err := exec.Command("git", "-c", "core.quotePath=false", "diff", "-U0", "-M", "--no-color", "--no-ext-diff",
		"--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--relative", "--diff-filter=AMR", ref+"...HEAD").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("git diff %s...HEAD: %s", ref, gitError(nil, err))
	}
	files, renames := diffLines(string(out))
	changed, renamed = map[string]ChangedFunctions{}, map[string]string{}
	for name, lines := range files {
		spec := lang.Detect(name)
		if spec == nil {
			continue
		}
		abs, err := filepath.Abs(filepath.FromSlash(name))
		if err != nil {
			return nil, nil, err
		}
		if old, ok := renames[name]; ok {
			if renamed[abs], err = filepath.Abs(filepath.FromSlash(old)); err != nil {
				return nil, nil, err
			}
		}
		// ./ makes the path relative to the working directory, as --relative
		// made name.
		src, err := exec.Command("git", "cat-file", "blob", "HEAD:./"+name).Output()
		if err != nil {
			return nil, nil, fmt.Errorf("git cat-file HEAD:./%s: %s", name, gitError(nil, err))
		}
		f, err := lang.Parse(spec, abs, src)
		if err != nil {
			return nil, nil, err
		}
		c := ChangedFunctions{Functions: map[string]bool{}}
		named := map[string]int{}
		for _, u := range f.Units {
			named[u.Namespace+"#"+u.Name]++
			if overlaps(lines, u.StartLine, u.EndLine) {
				c.Functions[u.Namespace+"#"+u.Name] = true
			}
		}
		for _, u := range f.Units {
			id := u.Namespace + "#" + u.Name
			if hash == nil || named[id] < 2 || !c.Functions[id] || overlaps(lines, u.StartLine, u.EndLine) {
				continue
			}
			if c.Kept == nil {
				c.Kept = map[string]map[string]bool{}
			}
			if c.Kept[id] == nil {
				c.Kept[id] = map[string]bool{}
			}
			c.Kept[id][hash(f, u)] = true
		}
		f.Close()
		changed[abs] = c
	}
	return changed, renamed, nil
}

// ChangedFunctions is what a range of commits changed in one file.
type ChangedFunctions struct {
	// Functions holds the namespace#name of each function whose lines the
	// range touched.
	Functions map[string]bool
	// Kept holds, for a name in Functions that several functions of the
	// file share, the hash of each of them the range left alone, as HEAD
	// has it.
	Kept map[string]map[string]bool
}

// Judges reports whether the function named function, whose text has hash,
// is one the range changed: by its name, and among functions sharing it, by
// its hash, so of several init functions only those the range touched are.
func (c ChangedFunctions) Judges(function, hash string) bool {
	return c.Functions[function] && !c.Kept[function][hash]
}

// lines is a range of line numbers, both ends included.
type lines struct{ start, end int }

func overlaps(ranges []lines, start, end int) bool {
	for _, r := range ranges {
		if r.start <= end && r.end >= start {
			return true
		}
	}
	return false
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
				out[to] = out[to]
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
		return lines{start, start + 1}, true
	}
	return lines{start, start + count - 1}, true
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
