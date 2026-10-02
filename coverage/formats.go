package coverage

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// Load reads a report, choosing the format from its content: a Go cover
// profile starts with "mode:", JaCoCo and Kover write XML, and anything else
// is read as LCOV.
func Load(file string) ([]Entry, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	head, _ := r.Peek(256)
	text := strings.TrimSpace(string(head))
	switch {
	case strings.HasPrefix(text, "mode:"):
		return ParseGo(r)
	case strings.HasPrefix(text, "<"):
		return ParseJaCoCo(r)
	default:
		return ParseLCOV(r)
	}
}

// ParseLCOV reads SF/DA records. Each measured line is a segment of weight
// one.
func ParseLCOV(r io.Reader) ([]Entry, error) {
	var entries []Entry
	var cur *Entry
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		switch {
		case strings.HasPrefix(line, "SF:"):
			entries = append(entries, Entry{Path: strings.TrimPrefix(line, "SF:")})
			cur = &entries[len(entries)-1]
		case strings.HasPrefix(line, "DA:") && cur != nil:
			fields := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(fields) < 2 {
				continue
			}
			n, err1 := strconv.Atoi(fields[0])
			hits, err2 := strconv.ParseFloat(fields[1], 64)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("lcov: bad line %q", line)
			}
			cur.Segments = append(cur.Segments, Segment{Start: n, End: n, Total: 1, Covered: boolWeight(hits > 0, 1)})
		case line == "end_of_record":
			cur = nil
		}
	}
	return entries, s.Err()
}

// ParseGo reads a `go test -coverprofile` file. Each block is a segment
// weighted by its statement count.
func ParseGo(r io.Reader) ([]Entry, error) {
	byFile := map[string]int{}
	var entries []Entry
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// path/file.go:12.34,15.2 3 1
		colon := strings.LastIndex(line, ":")
		fields := strings.Fields(line[colon+1:])
		if colon < 0 || len(fields) != 3 {
			return nil, fmt.Errorf("go cover: bad line %q", line)
		}
		start, end, ok := goSpan(fields[0])
		stmts, err1 := strconv.ParseFloat(fields[1], 64)
		count, err2 := strconv.ParseFloat(fields[2], 64)
		if !ok || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("go cover: bad line %q", line)
		}
		file := line[:colon]
		i, seen := byFile[file]
		if !seen {
			i = len(entries)
			byFile[file] = i
			entries = append(entries, Entry{Path: file})
		}
		entries[i].Segments = append(entries[i].Segments,
			Segment{Start: start, End: end, Total: stmts, Covered: boolWeight(count > 0, stmts)})
	}
	return entries, s.Err()
}

func goSpan(span string) (start, end int, ok bool) {
	from, to, found := strings.Cut(span, ",")
	if !found {
		return 0, 0, false
	}
	sl, _, _ := strings.Cut(from, ".")
	el, _, _ := strings.Cut(to, ".")
	start, err1 := strconv.Atoi(sl)
	end, err2 := strconv.Atoi(el)
	return start, end, err1 == nil && err2 == nil
}

// ParseJaCoCo reads JaCoCo XML, which Kover also writes. Each line is a
// segment weighted by its instructions, the counter JaCoCo is built around.
func ParseJaCoCo(r io.Reader) ([]Entry, error) {
	var report struct {
		Packages []struct {
			Name  string `xml:"name,attr"`
			Files []struct {
				Name  string `xml:"name,attr"`
				Lines []struct {
					Nr int     `xml:"nr,attr"`
					Mi float64 `xml:"mi,attr"`
					Ci float64 `xml:"ci,attr"`
				} `xml:"line"`
			} `xml:"sourcefile"`
		} `xml:"package"`
	}
	dec := xml.NewDecoder(r)
	// JaCoCo reports reference a DTD the decoder must not fetch.
	dec.Strict = false
	if err := dec.Decode(&report); err != nil {
		return nil, fmt.Errorf("jacoco: %w", err)
	}
	var entries []Entry
	for _, p := range report.Packages {
		for _, f := range p.Files {
			e := Entry{Path: path.Join(p.Name, f.Name)}
			for _, l := range f.Lines {
				e.Segments = append(e.Segments, Segment{Start: l.Nr, End: l.Nr, Total: l.Mi + l.Ci, Covered: l.Ci})
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func boolWeight(b bool, w float64) float64 {
	if b {
		return w
	}
	return 0
}
