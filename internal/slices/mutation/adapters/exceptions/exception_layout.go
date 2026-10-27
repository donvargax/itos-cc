package exceptions

import (
	"bytes"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/settings"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// listEdit is what a command changes in mutation.exceptions, n entries
// long as itos-cc.yaml has it: the entries it replaces, by index, those it
// removes, and those it adds after the last.
type listEdit struct {
	n       int
	replace map[int]mutants.Exception
	remove  map[int]bool
	add     []mutants.Exception
}

// result is the list as the edit leaves it.
func (l listEdit) result(entries []mutants.Exception) []mutants.Exception {
	var out []mutants.Exception
	for i, e := range entries {
		if l.remove[i] {
			continue
		}
		if r, ok := l.replace[i]; ok {
			e = r
		}
		out = append(out, e)
	}
	return append(out, l.add...)
}

func (l listEdit) empty() bool { return len(l.replace) == 0 && len(l.remove) == 0 && len(l.add) == 0 }

// writeList writes edit into data, whose list settings.Parse read as entries
// and doc is the node tree of. It splices text at the places yaml.v3 gives
// the list and its entries, so every byte but the lines of the entries it
// adds, replaces or removes is kept (splice); a layout it cannot splice, such
// as a flow-style mutation mapping, or a splice that does not read back as
// the edit's list with the rest of the document unchanged, is written by
// re-encoding the whole document, as before (encodeList).
func writeList(data []byte, doc *yaml.Node, entries []mutants.Exception, edit listEdit) ([]byte, error) {
	if edit.empty() {
		return data, nil
	}
	want := edit.result(entries)
	if out, ok := splice(data, doc, edit); ok && readsAs(out, data, want) {
		return out, nil
	}
	return encodeList(doc, edit)
}

// readsAs says whether out reads as want under mutation.exceptions, and as
// data everywhere else.
func readsAs(out, data []byte, want []mutants.Exception) bool {
	c, err := settings.Parse(out)
	if err != nil || !slices.Equal(c.Exceptions, want) {
		return false
	}
	before, err1 := withoutList(data)
	after, err2 := withoutList(out)
	return err1 == nil && err2 == nil && reflect.DeepEqual(before, after)
}

// withoutList is the document data holds, as JSON's data model, less
// mutation.exceptions, and less mutation when nothing else is in it.
func withoutList(data []byte) (any, error) {
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	root, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	if m, ok := root["mutation"].(map[string]any); ok {
		delete(m, "exceptions")
		if len(m) == 0 {
			delete(root, "mutation")
		}
	} else if root["mutation"] == nil {
		delete(root, "mutation")
	}
	if len(root) == 0 {
		return nil, nil
	}
	return root, nil
}

// encodeList writes edit by re-encoding the whole document, each entry it
// keeps or replaces keeping its comments, and each it replaces its x- keys,
// each it writes quoting as the document does (quoteOf).
func encodeList(doc *yaml.Node, edit listEdit) ([]byte, error) {
	quote := quoteOf(doc)
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null" {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	list := valueOf(valueOf(doc.Content[0], "mutation", yaml.MappingNode), "exceptions", yaml.SequenceNode)
	var content []*yaml.Node
	for i, was := range list.Content {
		if edit.remove[i] {
			continue
		}
		if e, ok := edit.replace[i]; ok {
			entry, err := entryNode(e, quote)
			if err != nil {
				return nil, err
			}
			entry.Content = append(entry.Content, extensions(was.Content)...)
			entry.HeadComment, entry.LineComment, entry.FootComment = was.HeadComment, was.LineComment, was.FootComment
			was = &entry
		}
		content = append(content, was)
	}
	for _, e := range edit.add {
		entry, err := entryNode(e, quote)
		if err != nil {
			return nil, err
		}
		content = append(content, &entry)
	}
	list.Content = content
	return encodeDocument(doc)
}

// text is a document's bytes by line, lines counted from 1 and columns in
// characters, as yaml.v3 counts them, with the line break and the quote the
// entries written into it take.
type text struct {
	data   []byte
	starts []int // the offset of each line
	eol    string
	quote  yaml.Style // the quote of the strings it writes that need one (quoteOf)
}

func newText(data []byte) *text {
	t := &text{data: data, eol: "\n"}
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		t.eol = "\r\n"
	}
	if len(data) > 0 {
		t.starts = append(t.starts, 0)
	}
	for i, b := range data {
		if b == '\n' && i+1 < len(data) {
			t.starts = append(t.starts, i+1)
		}
	}
	return t
}

// lines is how many lines the text has.
func (t *text) lines() int { return len(t.starts) }

// start is the offset of line, or the end of the text past its last line.
func (t *text) start(line int) int {
	if line > len(t.starts) {
		return len(t.data)
	}
	return t.starts[line-1]
}

// end is the offset after line, past its line break.
func (t *text) end(line int) int { return t.start(line + 1) }

// line is line's text, less its line break.
func (t *text) line(line int) string {
	return strings.TrimRight(string(t.data[t.start(line):t.end(line)]), "\r\n")
}

// offset is the offset of column on line, false when the line is shorter.
func (t *text) offset(line, column int) (int, bool) {
	if line < 1 || line > t.lines() || column < 1 {
		return 0, false
	}
	at := t.start(line)
	for range column - 1 {
		if at >= t.end(line) {
			return 0, false
		}
		_, size := utf8.DecodeRune(t.data[at:])
		at += size
	}
	return at, true
}

// documentEnd is the first line from line on that ends the document, a
// "---" or "..." at its start, or the line past the last.
func (t *text) documentEnd(line int) int {
	for ; line <= t.lines(); line++ {
		l := t.line(line)
		for _, marker := range []string{"---", "..."} {
			if l == marker || strings.HasPrefix(l, marker+" ") || strings.HasPrefix(l, marker+"\t") {
				return line
			}
		}
	}
	return t.lines() + 1
}

// lastLine is the last line of the node that starts at offset from on
// line first and whose lines are indented by dedent or more, below limit:
// the least prefix of those lines that reads as want, dedented, so the
// comments and blank lines after the node are not its own. The lines that
// read as want are the node and any after it up to limit, so a binary
// search finds it.
func (t *text) lastLine(from, first, dedent, limit int, want any) (int, bool) {
	reads := func(last int) bool {
		var b strings.Builder
		for _, l := range strings.SplitAfter(string(t.data[from:t.end(last)]), "\n") {
			n := len(l) - len(strings.TrimLeft(l, " "))
			b.WriteString(l[min(n, dedent):])
		}
		var got any
		return yaml.Unmarshal([]byte(b.String()), &got) == nil && reflect.DeepEqual(got, want)
	}
	if limit <= first || !reads(limit-1) {
		return 0, false
	}
	n := sort.Search(limit-first, func(i int) bool { return reads(first + i) })
	return first + n, true
}

// nodes is every node of doc in document order.
func nodes(doc *yaml.Node) []*yaml.Node {
	var out []*yaml.Node
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		out = append(out, n)
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// quoteOf is the quote the strings a writer adds to doc take when they
// need one, the quote doc already uses (slice-66, issue #64):
// yaml.DoubleQuotedStyle when it holds more double-quoted scalars than
// single-quoted ones, any key or value, else 0, the codec's own choice,
// which is single quotes.
func quoteOf(doc *yaml.Node) yaml.Style {
	lead := 0 // how many more double-quoted scalars doc holds than single-quoted
	for _, n := range nodes(doc) {
		switch {
		case n.Style&yaml.DoubleQuotedStyle != 0:
			lead++
		case n.Style&yaml.SingleQuotedStyle != 0:
			lead--
		}
	}
	if lead > 0 {
		return yaml.DoubleQuotedStyle
	}
	return 0
}

// entryNode is e as the codec encodes it, each string it would write in
// single quotes written in double quotes when quote is
// yaml.DoubleQuotedStyle; a string that needs no quotes stays plain.
func entryNode(e mutants.Exception, quote yaml.Style) (yaml.Node, error) {
	var entry yaml.Node
	err := entry.Encode(e)
	if quote != yaml.DoubleQuotedStyle {
		return entry, err
	}
	for _, n := range entry.Content {
		// yaml.v3 quotes a scalar written alone as it does a block mapping's,
		// and cannot fail to write one.
		if out, _ := yaml.Marshal(n); bytes.HasPrefix(out, []byte("'")) {
			n.Style = yaml.DoubleQuotedStyle
		}
	}
	return entry, err
}

// after is the line of the first node after n and all it holds, or the end
// of the document from n on.
func (t *text) after(all []*yaml.Node, n *yaml.Node) int {
	end := t.documentEnd(n.Line)
	i := slices.Index(all, n)
	if i < 0 {
		return end
	}
	inside := len(nodes(n))
	if i+inside < len(all) {
		return min(end, all[i+inside].Line)
	}
	return end
}

// change is a change of the text: the bytes from start to end become with.
type change struct {
	start, end int
	with       string
}

func apply(data []byte, edits []change) []byte {
	slices.SortFunc(edits, func(a, b change) int { return b.start - a.start })
	out := slices.Clone(data)
	for _, e := range edits {
		out = slices.Concat(out[:e.start], []byte(e.with), out[e.end:])
	}
	return out
}

// isExtension says whether k is an extension key (settings.Extension).
func isExtension(k *yaml.Node) bool { return strings.HasPrefix(k.Value, settings.Extension) }

// extensions are the x- keys of the mapping whose content is pairs, each
// followed by its value, in their order: the keys an entry a writer
// rewrites keeps after the codec's (slice-55).
func extensions(pairs []*yaml.Node) []*yaml.Node {
	var out []*yaml.Node
	for i := 0; i+1 < len(pairs); i += 2 {
		if isExtension(pairs[i]) {
			out = append(out, pairs[i], pairs[i+1])
		}
	}
	return out
}

// detached is a copy of ns and all they hold without their foot comments,
// which yaml.v3 gives the comment lines after a node: a splice keeps those
// lines where they are, past the entry's own, so a copy must not repeat
// them.
func detached(ns []*yaml.Node) []*yaml.Node {
	out := make([]*yaml.Node, len(ns))
	for i, n := range ns {
		c := *n
		c.FootComment, c.Content = "", detached(n.Content)
		out[i] = &c
	}
	return out
}

// entryText is e as a block entry of a list whose dashes are at column
// dash, its keys in the codec's order, quoted as t.quote says (entryNode),
// then ext, the x- keys of the entry it rewrites and their values
// (extensions), with the comments yaml.v3 gives them, the lines directly
// above each included; each line ends with t.eol.
func (t *text) entryText(e mutants.Exception, ext []*yaml.Node, dash int) (string, bool) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	entry, err := entryNode(e, t.quote)
	if err != nil || enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Content: append(entry.Content, ext...)}) != nil || enc.Close() != nil {
		return "", false
	}
	eol := t.eol
	var b strings.Builder
	for i, l := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		switch {
		case i == 0:
			b.WriteString(strings.Repeat(" ", dash-1) + "- " + l)
		case l != "":
			b.WriteString(strings.Repeat(" ", dash+1) + l)
		}
		b.WriteString(eol)
	}
	return b.String(), true
}

// entriesText is entries as block entries (entryText), each the first of
// exts, by index, as its x- keys, none past them.
func (t *text) entriesText(entries []mutants.Exception, exts [][]*yaml.Node, dash int) (string, bool) {
	var b strings.Builder
	for i, e := range entries {
		var ext []*yaml.Node
		if i < len(exts) {
			ext = exts[i]
		}
		s, ok := t.entryText(e, ext, dash)
		if !ok {
			return "", false
		}
		b.WriteString(s)
	}
	return b.String(), true
}

// key is the key and value of key in the block mapping m, nil when it has
// none.
func key(m *yaml.Node, name string) (k, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Kind == yaml.ScalarNode && m.Content[i].Value == name {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func isNull(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.Tag == "!!null" }
func isFlow(n *yaml.Node) bool { return n.Style&yaml.FlowStyle != 0 }

// splice writes edit into data by changing only the lines of the entries
// it touches; false when data's layout is one it does not splice.
func splice(data []byte, doc *yaml.Node, edit listEdit) ([]byte, bool) {
	t := newText(data)
	t.quote = quoteOf(doc)
	all := nodes(doc)
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return t.appendTo(t.documentEnd(1), 0, "mutation", edit)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || isFlow(root) {
		return nil, false
	}
	mk, mv := key(root, "mutation")
	switch {
	case mk == nil:
		return t.appendTo(t.documentEnd(root.Line), root.Column-1, "mutation", edit)
	case isNull(mv):
		return t.fillNull(mk, mv, "exceptions", edit)
	case mv.Kind != yaml.MappingNode || isFlow(mv):
		return nil, false
	}
	ek, ev := key(mv, "exceptions")
	switch {
	case ek == nil:
		var want any
		if mv.Decode(&want) != nil {
			return nil, false
		}
		from := t.start(mv.Line)
		last, ok := t.lastLine(from, mv.Line, mv.Column-1, t.after(all, mv), want)
		if !ok {
			return nil, false
		}
		return t.appendTo(last+1, mv.Column-1, "exceptions", edit)
	case isNull(ev):
		return t.fillNull(ek, ev, "", edit)
	case ev.Kind != yaml.SequenceNode:
		return nil, false
	case isFlow(ev):
		return t.rewriteFlow(all, ek, ev, edit)
	}
	return t.spliceBlock(all, ek, ev, edit)
}

// appendTo adds edit's entries, to a list that has none, at the start of
// line, under the keys from name down, the first at indent.
func (t *text) appendTo(line, indent int, name string, edit listEdit) ([]byte, bool) {
	if edit.n != 0 || len(edit.add) == 0 {
		return nil, false
	}
	pad := strings.Repeat(" ", indent)
	var head string
	if name == "mutation" {
		head = pad + "mutation:" + t.eol
		pad += "  "
	}
	head += pad + "exceptions:" + t.eol
	entries, ok := t.entriesText(edit.add, nil, len(pad)+3)
	if !ok {
		return nil, false
	}
	at := t.start(line)
	if at > 0 && t.data[at-1] != '\n' {
		head = t.eol + head
	}
	return apply(t.data, []change{{at, at, head + entries}}), true
}

// fillNull gives the key k, whose value v is null, the list of edit's
// entries under it, two spaces in, after child when it names the key the
// list is under.
func (t *text) fillNull(k, v *yaml.Node, child string, edit listEdit) ([]byte, bool) {
	if edit.n != 0 || len(edit.add) == 0 {
		return nil, false
	}
	var edits []change
	if v.Value != "" {
		start, ok1 := t.offset(v.Line, v.Column)
		if !ok1 || v.Line != k.Line {
			return nil, false
		}
		for start > t.start(v.Line) && (t.data[start-1] == ' ' || t.data[start-1] == '\t') {
			start--
		}
		end, ok2 := t.offset(v.Line, v.Column+utf8.RuneCountInString(v.Value))
		if !ok2 {
			return nil, false
		}
		edits = append(edits, change{start, end, ""})
	}
	pad := k.Column - 1
	var head string
	if child != "" {
		pad += 2
		head = strings.Repeat(" ", pad) + child + ":" + t.eol
	}
	entries, ok := t.entriesText(edit.add, nil, pad+3)
	if !ok {
		return nil, false
	}
	at := t.end(k.Line)
	if at > 0 && t.data[at-1] != '\n' {
		head = t.eol + head
	}
	return apply(t.data, append(edits, change{at, at, head + entries})), true
}

// rewriteFlow writes the flow list v, the value of k, whole as a block
// list of the entries edit leaves, two spaces under k, each it keeps or
// replaces keeping its x- keys.
func (t *text) rewriteFlow(all []*yaml.Node, k, v *yaml.Node, edit listEdit) ([]byte, bool) {
	var want any
	if v.Decode(&want) != nil || len(v.Content) != edit.n {
		return nil, false
	}
	from, ok := t.offset(v.Line, v.Column)
	if !ok {
		return nil, false
	}
	last, ok := t.lastLine(from, v.Line, 0, t.after(all, v), want)
	if !ok {
		return nil, false
	}
	var entries []mutants.Exception
	var exts [][]*yaml.Node
	for i, n := range v.Content {
		var e mutants.Exception
		if n.Decode(&e) != nil {
			return nil, false
		}
		entries = append(entries, e)
		if !edit.remove[i] {
			exts = append(exts, detached(extensions(n.Content)))
		}
	}
	with := " []" + t.eol
	if list := edit.result(entries); len(list) > 0 {
		s, ok := t.entriesText(list, exts, k.Column+2)
		if !ok {
			return nil, false
		}
		with = t.eol + s
	}
	for from > t.start(v.Line) && (t.data[from-1] == ' ' || t.data[from-1] == '\t') {
		from--
	}
	end := t.end(last)
	if end == len(t.data) && !bytes.HasSuffix(t.data, []byte("\n")) {
		with = strings.TrimSuffix(with, t.eol)
	}
	return apply(t.data, []change{{from, end, with}}), true
}

// spliceBlock writes edit into the block list seq entry by entry: a
// replaced entry's lines become the new entry's, keeping the comment after
// its first line and its x- keys; a removed entry's lines go, with the
// comment lines directly above it; and added entries follow the last.
func (t *text) spliceBlock(all []*yaml.Node, k, seq *yaml.Node, edit listEdit) ([]byte, bool) {
	if len(seq.Content) != edit.n {
		return nil, false
	}
	type span struct{ dash, last int }
	spans := make([]span, len(seq.Content))
	for i, n := range seq.Content {
		dash, ok := t.dashLine(seq, n)
		if !ok {
			return nil, false
		}
		spans[i].dash = dash
	}
	for i, n := range seq.Content {
		var v any
		if n.Decode(&v) != nil {
			return nil, false
		}
		limit := t.after(all, seq)
		if i+1 < len(spans) {
			limit = spans[i+1].dash
		}
		last, ok := t.lastLine(t.start(spans[i].dash), spans[i].dash, seq.Column-1, limit, []any{v})
		if !ok {
			return nil, false
		}
		spans[i].last = last
	}
	var edits []change
	for i, n := range seq.Content {
		s := spans[i]
		end := t.end(s.last)
		switch e, replaced := edit.replace[i]; {
		case edit.remove[i]:
			start := s.dash
			floor := k.Line + 1
			if i > 0 {
				floor = spans[i-1].last + 1
			}
			for start-1 >= floor && strings.HasPrefix(strings.TrimSpace(t.line(start-1)), "#") {
				start--
			}
			edits = append(edits, change{t.start(start), end, ""})
		case replaced:
			with, ok := t.entryText(e, detached(extensions(n.Content)), seq.Column)
			if !ok {
				return nil, false
			}
			if c := t.firstLineComment(n, s.dash); c != "" {
				first, rest, _ := strings.Cut(with, t.eol)
				with = first + c + t.eol + rest
			}
			if end == len(t.data) && !bytes.HasSuffix(t.data, []byte("\n")) {
				with = strings.TrimSuffix(with, t.eol)
			}
			edits = append(edits, change{t.start(s.dash), end, with})
		}
	}
	if len(edit.add) > 0 {
		with, ok := t.entriesText(edit.add, nil, seq.Column)
		if !ok {
			return nil, false
		}
		at := t.end(spans[len(spans)-1].last)
		if at > 0 && t.data[at-1] != '\n' {
			with = t.eol + with
		}
		edits = append(edits, change{at, at, with})
	}
	return apply(t.data, edits), true
}

// dashLine is the line of the dash of entry n of the block list seq: the
// nearest line at or above n's own whose text at seq's column is "-",
// with only spaces before it.
func (t *text) dashLine(seq, n *yaml.Node) (int, bool) {
	for line := n.Line; line >= seq.Line && line >= 1; line-- {
		l := t.line(line)
		at := seq.Column - 1
		if len(l) > at && l[at] == '-' && strings.TrimLeft(l[:at], " ") == "" && (len(l) == at+1 || l[at+1] == ' ' || l[at+1] == '\t') {
			return line, true
		}
	}
	return 0, false
}

// firstLineComment is the comment after the first line of entry n, whose
// dash is on line, with the space before it, or "". The comment of an x-
// key or its value is not it, as it goes with that key.
func (t *text) firstLineComment(n *yaml.Node, line int) string {
	own := []*yaml.Node{n}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if !isExtension(n.Content[i]) {
			own = append(own, nodes(n.Content[i])...)
			own = append(own, nodes(n.Content[i+1])...)
		}
	}
	for _, c := range own {
		if c.Line != line || c.LineComment == "" {
			continue
		}
		l := t.line(line)
		at := strings.LastIndex(l, c.LineComment)
		if at < 0 {
			return ""
		}
		for at > 0 && (l[at-1] == ' ' || l[at-1] == '\t') {
			at--
		}
		return l[at:]
	}
	return ""
}
