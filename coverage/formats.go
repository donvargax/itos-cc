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

// ParseLCOV reads SF/DA records, where each measured line is a segment of
// weight one, and BRDA records, where each block with two or more branches is
// a decision. A block with one branch is not a choice: c8, Node, and Vitest's
// v8 provider before its AST-aware remapping write one for every V8 block
// whose count differs from the code around it, function bodies included, and
// leave out the arm that ran as often as its parent.
func ParseLCOV(r io.Reader) ([]Entry, error) {
	var entries []Entry
	var cur *Entry
	var blocks lcovBlocks
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		switch {
		case strings.HasPrefix(line, "SF:"):
			blocks.flush(cur)
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
			cur.Segments = append(cur.Segments, Segment{Start: n, End: n, Total: 1, Covered: boolWeight(hits > 0, 1), Key: fields[0]})
		case strings.HasPrefix(line, "BRDA:") && cur != nil:
			if err := blocks.add(strings.TrimPrefix(line, "BRDA:")); err != nil {
				return nil, err
			}
		case line == "end_of_record":
			blocks.flush(cur)
			cur = nil
		}
	}
	blocks.flush(cur)
	return entries, s.Err()
}

// lcovBlocks gathers one file's BRDA records by line and block, in order.
type lcovBlocks struct {
	order []Segment
	index map[[2]string]int
}

// add reads "line,block,branch,taken". taken is "-" when the line never
// ran. coverage.py describes the branch in words, so only the first two
// fields and the last are read.
func (b *lcovBlocks) add(record string) error {
	first := strings.Index(record, ",")
	last := strings.LastIndex(record, ",")
	if first < 0 {
		return fmt.Errorf("lcov: bad branch %q", record)
	}
	ln, err := strconv.Atoi(record[:first])
	block, _, _ := strings.Cut(record[first+1:], ",")
	if err != nil || last <= first {
		return fmt.Errorf("lcov: bad branch %q", record)
	}
	taken, _ := strconv.ParseFloat(record[last+1:], 64)
	key := [2]string{record[:first], block}
	if b.index == nil {
		b.index = map[[2]string]int{}
	}
	i, ok := b.index[key]
	if !ok {
		i = len(b.order)
		b.index[key] = i
		b.order = append(b.order, Segment{Start: ln, End: ln, Key: key[0] + "," + key[1]})
	}
	b.order[i].Total++
	b.order[i].Covered += boolWeight(taken > 0, 1)
	return nil
}

// flush gives cur the decisions gathered so far and starts over.
func (b *lcovBlocks) flush(cur *Entry) {
	if cur != nil {
		for _, s := range b.order {
			if s.Total >= 2 {
				cur.Branches = append(cur.Branches, s)
			}
		}
	}
	*b = lcovBlocks{}
}

// ParseGo reads a `go test -coverprofile` file. Each block is a segment
// weighted by its statement count and keyed by its span. With -coverpkg,
// every test binary lists every block; Build counts each block once.
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
			Segment{Start: start, End: end, Total: stmts, Covered: boolWeight(count > 0, stmts), Key: fields[0]})
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
// segment weighted by its instructions, the counter JaCoCo is built around,
// and a line with branches is also a decision weighted by them.
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
					Mb float64 `xml:"mb,attr"`
					Cb float64 `xml:"cb,attr"`
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
				e.Segments = append(e.Segments, Segment{Start: l.Nr, End: l.Nr, Total: l.Mi + l.Ci, Covered: l.Ci, Key: strconv.Itoa(l.Nr)})
				if l.Mb+l.Cb > 0 {
					e.Branches = append(e.Branches, Segment{Start: l.Nr, End: l.Nr, Total: l.Mb + l.Cb, Covered: l.Cb, Key: strconv.Itoa(l.Nr)})
				}
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
