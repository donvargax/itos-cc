package coverage

import (
	"context"

	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// What the runner records of a Report as it measures: the domain's own
// setters, so it builds Reports through them and never through their
// fields (decision 47).

// SetMissing records ms as what was meant to be measured and was not
// (Missing), in place of what r held: nothing when ms is empty.
func (r *Report) SetMissing(ms ...Unmeasured) {
	r.missing = ms
}

// SetIntegrationMissing records ms as each build root whose processes the
// tests started went unmeasured (IntegrationMissing), in place of what r
// held.
func (r *Report) SetIntegrationMissing(ms ...Unmeasured) {
	r.integrationMissing = ms
}

// IntegrationMissing is each build root whose collector could not measure
// the processes its tests start, such as a coverage.py that does not start
// in them: its in-process coverage stands, without integration coverage.
func (r *Report) IntegrationMissing() []Unmeasured {
	if r == nil {
		return nil
	}
	return r.integrationMissing
}

// MarkMeasured records that a coverage command of language ran
// successfully: every command exited 0 and wrote its report (Measures).
func (r *Report) MarkMeasured(language string) {
	if r.languages == nil {
		r.languages = map[string]bool{}
	}
	r.languages[language] = true
}

// Prove records, of the report of a plan of language in its build root dir
// whose commands all succeeded and wrote their reports, which of sources it
// lists completely, at its format's line precision (Lines), and which no
// test loaded, so it names none of their lines (Unloaded), in place of what
// r held.
func (r *Report) Prove(language, dir string, sources []string) {
	r.proven, r.unloaded = map[string]bool{}, map[string]Unloaded{}
	for _, s := range sources {
		if r.Has(s) {
			r.proven[s] = true
		} else {
			r.unloaded[s] = Unloaded{Language: language, Dir: dir}
		}
	}
}

// Empty reports whether r names no file at all.
func (r *Report) Empty() bool {
	return r == nil || len(r.files) == 0
}

// UnloadedByDir is Unloaded's files of language by the build root of the
// plan that measured them, each list sorted.
func (r *Report) UnloadedByDir(language string) map[string][]string {
	byDir := map[string][]string{}
	for _, file := range r.Unloaded(language) {
		dir := r.unloaded[file].Dir
		byDir[dir] = append(byDir[dir], file)
	}
	return byDir
}

// List records, of sources no test loaded, the executable lines entries
// name, a report's paths relative to dir, all of them unexecuted, and
// proves each file it names complete (Lines): a static inventory's, or the
// report of a module that lists every class of it. Its paths match as
// Build's do, files answering what is on disk.
func (r *Report) List(ctx context.Context, files port.Files, sources []string, dir string, entries []Entry) {
	m := newMatcher(sources)
	for _, e := range entries {
		file := m.match(ctx, files, e.Path, dir)
		if file == "" {
			continue
		}
		if r.inventory == nil {
			r.inventory = map[string][]Segment{}
		}
		if r.proven == nil {
			r.proven = map[string]bool{}
		}
		for _, seg := range e.Segments {
			seg.Covered, seg.ran = 0, false
			r.inventory[file] = append(r.inventory[file], seg)
		}
		r.proven[file] = true
	}
}

// Integrated is entries, what the processes the tests started executed,
// marked Integration, each of a file r names kept to the pieces r counts
// as executable there: integration data covers what the in-process report
// counts in a file it names, and adds nothing to it, since a collector of
// another tool, such as c8 beside Vitest's v8 provider, names other lines,
// blank and brace lines among them. Its paths match sources as Build's do,
// from base, files answering what is on disk.
func (r *Report) Integrated(ctx context.Context, files port.Files, sources []string, base string, entries []Entry) []Entry {
	m := newMatcher(sources)
	for i := range entries {
		entries[i].Source = Integration
		if file := m.match(ctx, files, entries[i].Path, base); file != "" && r.Has(file) {
			entries[i].Segments = keyedLike(entries[i].Segments, r.files[file])
			entries[i].Branches = keyedLike(entries[i].Branches, r.branches[file])
		}
	}
	return entries
}

// keyedLike is the segments of segs whose key one of like holds, and
// those with no key.
func keyedLike(segs, like []Segment) []Segment {
	keys := map[string]bool{}
	for _, s := range like {
		keys[s.Key] = true
	}
	var out []Segment
	for _, s := range segs {
		if s.Key == "" || keys[s.Key] {
			out = append(out, s)
		}
	}
	return out
}
