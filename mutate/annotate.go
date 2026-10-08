package mutate

import (
	"fmt"
	"os"
	"strings"

	"github.com/donvargax/itos-cc/lang"
)

const (
	annotationStart = "itos-cc mutation:"
	annotationEnd   = "end itos-cc mutation"
)

// annotationMarkers are the opening and closing markers a summary comment
// may have: the current ones, then those an earlier itos-cc wrote, named
// after the mutate command that became mutation run, so a file annotated
// then still gets one comment, never two.
var annotationMarkers = [][2]string{
	{annotationStart, annotationEnd},
	{"itos-cc mutate:", "end itos-cc mutate"},
}

// annotate replaces the summary comment at the end of path with one for
// snap, each survivor excepted by its reason in excepted, writing the file
// only when the summary changed. The comment sits after every function, so
// it never changes a function's hash.
func annotate(path string, spec *lang.Spec, snap Snapshot, excepted map[string]string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := StripAnnotation(string(src), spec.Comment)
	block := Annotation(snap, spec.Comment, excepted)
	updated := strings.TrimRight(body, "\n") + "\n\n" + block
	if block == "" {
		updated = strings.TrimRight(body, "\n") + "\n"
	}
	if updated == string(src) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), info.Mode().Perm())
}

// Annotation is the summary comment for snap: totals, then each survivor,
// which is what a reader can act on, then each survivor itos-cc.yaml
// excepts, with its reason. excepted holds those reasons by exceptedKey; a
// survivor with none is listed as survived. "excepted" is counted only
// where there is one, as mutation run's summary line counts it, so a file
// with none reads as it always did. A file without sites gets none.
func Annotation(snap Snapshot, comment string, excepted map[string]string) string {
	var killed, survived, uncovered int
	var survivors, exceptions []string
	for _, u := range snap.Units {
		killed += u.Killed
		survived += u.Survived
		uncovered += u.Uncovered
		for _, m := range u.Mutants {
			if m.Outcome != Survived {
				continue
			}
			site := fmt.Sprintf("line %d %s → %s in %s", m.Line, show(m.Original), show(m.Replacement), u.Name)
			if reason, ok := excepted[exceptedKey(unitID(u.Namespace, u.Name), u.Hash, m.key())]; ok {
				exceptions = append(exceptions, fmt.Sprintf("%s excepted: %s: %s", comment, site, oneLine(reason)))
				continue
			}
			survivors = append(survivors, fmt.Sprintf("%s survived: %s", comment, site))
		}
	}
	if killed+survived+uncovered == 0 {
		return ""
	}
	survived -= len(exceptions)
	counts := fmt.Sprintf("%d killed, %d survived, %d uncovered", killed, survived, uncovered)
	if len(exceptions) > 0 {
		counts = fmt.Sprintf("%d killed, %d survived, %d excepted, %d uncovered", killed, survived, len(exceptions), uncovered)
	}
	lines := []string{fmt.Sprintf("%s %s %s", comment, annotationStart, counts)}
	lines = append(lines, survivors...)
	lines = append(lines, exceptions...)
	lines = append(lines, comment+" "+annotationEnd)
	return strings.Join(lines, "\n") + "\n"
}

// oneLine is text on one line, each run of whitespace one space, so a
// reason written into the comment never leaves a line without its comment
// marker.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// StripAnnotation removes a trailing summary comment, if any, whichever
// markers it has (annotationMarkers).
func StripAnnotation(src, comment string) string {
	start, closing := -1, ""
	for _, m := range annotationMarkers {
		if at := strings.LastIndex(src, comment+" "+m[0]); at > start {
			start, closing = at, comment+" "+m[1]
		}
	}
	if start < 0 {
		return src
	}
	end := strings.Index(src[start:], closing)
	if end < 0 {
		return src
	}
	rest := strings.TrimLeft(src[start+end+len(closing):], "\r\n")
	if strings.TrimSpace(rest) != "" {
		// Code follows the block, so it is not ours to remove.
		return src
	}
	return strings.TrimRight(src[:start], "\n") + "\n"
}
