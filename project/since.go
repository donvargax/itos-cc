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

// ChangedSince returns the functions the commits since ref changed, by
// file: git diff ref...HEAD, so a branch's own commits whatever the base did
// since, and never the working tree. Each supported file under the working
// directory that the range changed is a key, by absolute path, even when no
// function of it changed; its value holds the namespace#name of each
// function whose lines the range touched. A deleted line counts as a change
// of the lines either side of it. Functions are found in HEAD's version of
// the file, so an uncommitted edit neither adds one nor moves its lines.
func ChangedSince(ref string) (map[string]map[string]bool, error) {
	if out, err := exec.Command("git", "rev-parse", "--git-dir").CombinedOutput(); err != nil {
		return nil, &NoGitError{gitError(out, err)}
	}
	// A ref is never read as an option of the git commands below.
	if strings.HasPrefix(ref, "-") || exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run() != nil {
		return nil, ErrBadRef
	}
	out, err := exec.Command("git", "-c", "core.quotePath=false", "diff", "-U0", "--no-color", "--no-ext-diff",
		"--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--relative", "--diff-filter=AMR", ref+"...HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s...HEAD: %s", ref, gitError(nil, err))
	}
	changed := map[string]map[string]bool{}
	for name, lines := range diffLines(string(out)) {
		spec := lang.Detect(name)
		if spec == nil {
			continue
		}
		abs, err := filepath.Abs(filepath.FromSlash(name))
		if err != nil {
			return nil, err
		}
		// ./ makes the path relative to the working directory, as --relative
		// made name.
		src, err := exec.Command("git", "cat-file", "blob", "HEAD:./"+name).Output()
		if err != nil {
			return nil, fmt.Errorf("git cat-file HEAD:./%s: %s", name, gitError(nil, err))
		}
		f, err := lang.Parse(spec, abs, src)
		if err != nil {
			return nil, err
		}
		functions := map[string]bool{}
		for _, u := range f.Units {
			if overlaps(lines, u.StartLine, u.EndLine) {
				functions[u.Namespace+"#"+u.Name] = true
			}
		}
		f.Close()
		changed[abs] = functions
	}
	return changed, nil
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
// prints it, unquoted.
func diffLines(diff string) map[string][]lines {
	out := map[string][]lines{}
	file, header := "", false
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file, header = "", true
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
	return out
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
