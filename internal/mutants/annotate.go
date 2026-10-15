package mutants

import (
	"fmt"

	"strings"
)

const (
	AnnotationStart = "itos-cc mutation:"
	AnnotationEnd   = "end itos-cc mutation"
)

// annotationMarkers are the opening and closing markers a summary comment
// may have: the current ones, then those an earlier itos-cc wrote, named
// after the mutate command that became mutation run, so a file annotated
// then still gets one comment, never two.
var AnnotationMarkers = [][2]string{
	{AnnotationStart, AnnotationEnd},
	{"itos-cc mutate:", "end itos-cc mutate"},
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
			site := fmt.Sprintf("line %d %s → %s in %s", m.Line, Show(m.Original), Show(m.Replacement), u.Name)
			if reason, ok := excepted[ExceptedKey(UnitID(u.Namespace, u.Name), u.Hash, m.Key())]; ok {
				exceptions = append(exceptions, fmt.Sprintf("%s excepted: %s: %s", comment, site, OneLine(reason)))
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
	lines := []string{fmt.Sprintf("%s %s %s", comment, AnnotationStart, counts)}
	lines = append(lines, survivors...)
	lines = append(lines, exceptions...)
	lines = append(lines, comment+" "+AnnotationEnd)
	return strings.Join(lines, "\n") + "\n"
}

// oneLine is text on one line, each run of whitespace one space, so a
// reason written into the comment never leaves a line without its comment
// marker.
func OneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// StripAnnotation removes a trailing summary comment, if any, whichever
// markers it has (annotationMarkers).
func StripAnnotation(src, comment string) string {
	start, closing := -1, ""
	for _, m := range AnnotationMarkers {
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
