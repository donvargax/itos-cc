package mutate

import (
	"fmt"
	"os"
	"strings"

	"itos-cc/lang"
)

const (
	annotationStart = "itos-cc mutate:"
	annotationEnd   = "end itos-cc mutate"
)

// annotate replaces the summary comment at the end of path with one for
// snap, writing the file only when the summary changed. The comment sits
// after every function, so it never changes a function's hash.
func annotate(path string, spec *lang.Spec, snap Snapshot) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := StripAnnotation(string(src), spec.Comment)
	block := Annotation(snap, spec.Comment)
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
// which is what a reader can act on. A file without sites gets none.
func Annotation(snap Snapshot, comment string) string {
	var killed, survived, uncovered int
	var survivors []string
	for _, u := range snap.Units {
		killed += u.Killed
		survived += u.Survived
		uncovered += u.Uncovered
		for _, m := range u.Mutants {
			if m.Outcome == Survived {
				survivors = append(survivors, fmt.Sprintf("%s survived: line %d %s → %s in %s",
					comment, m.Line, show(m.Original), show(m.Replacement), u.Name))
			}
		}
	}
	if killed+survived+uncovered == 0 {
		return ""
	}
	lines := []string{fmt.Sprintf("%s %s %d killed, %d survived, %d uncovered", comment, annotationStart, killed, survived, uncovered)}
	lines = append(lines, survivors...)
	lines = append(lines, comment+" "+annotationEnd)
	return strings.Join(lines, "\n") + "\n"
}

// StripAnnotation removes a trailing summary comment, if any.
func StripAnnotation(src, comment string) string {
	start := strings.LastIndex(src, comment+" "+annotationStart)
	if start < 0 {
		return src
	}
	end := strings.Index(src[start:], comment+" "+annotationEnd)
	if end < 0 {
		return src
	}
	rest := strings.TrimLeft(src[start+end+len(comment+" "+annotationEnd):], "\r\n")
	if strings.TrimSpace(rest) != "" {
		// Code follows the block, so it is not ours to remove.
		return src
	}
	return strings.TrimRight(src[:start], "\n") + "\n"
}
