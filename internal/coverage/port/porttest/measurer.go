package porttest

import (
	"context"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
)

// Measurer measures coverage in memory: the reports a request names are
// read from Files, and a request that names none is given the canned
// reports of Roots, or of Left when it reads what an earlier run left
// (Existing). It answers as internal/coveragetools' Measurer answers from
// the disk, which MeasurerContract holds; it runs no tool.
type Measurer struct {
	// Dir is the working directory, absolute: a relative path, a named
	// report or a key of Files, Roots and Left, is from it. A path rooted
	// with no volume, such as "/work", is on Dir's volume, as Files' are.
	Dir string
	// Files are the files on the disk, their text by path: a named report
	// is read from them, and each is a regular file, which a path a report
	// names may be.
	Files map[string]string
	// Roots are the reports the project's own tools measure, by the build
	// root they run in: a request that names no report is given the report
	// of each root that is the nearest to one of its sources, merged in the
	// roots' order, as a build root measures what is below it and a root
	// nested in another measures its own. A source under no root is
	// measured by none and missing from none.
	Roots map[string]*coverage.Report
	// Left are the reports an earlier run left, by build root, which a
	// request that reads them (Existing) is given as Roots are.
	Left map[string]*coverage.Report
}

var _ port.Measurer[*coverage.Report] = Measurer{}

// Measure gives the report of req, as the port says. A Command runs
// nothing here: what it writes is in Files already, and a report of it
// that is not there measured nothing.
func (m Measurer) Measure(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	if ctx.Err() != nil {
		return nil, port.Cancelled{Cause: context.Cause(ctx)}
	}
	switch {
	case len(req.Reports) == 0 && req.Existing:
		return m.canned(m.Left, req.Sources), nil
	case len(req.Reports) == 0:
		return m.canned(m.Roots, req.Sources), nil
	}
	var all [][]coverage.Entry
	var missing []coverage.Unmeasured
	for _, path := range req.Reports {
		entries, err := m.load(path)
		if err != nil {
			missing = append(missing, coverage.Unmeasured{Report: path, Cause: port.Unreadable{}, Reason: err.Error()})
			continue
		}
		all = append(all, entries)
	}
	r := coverage.Build(ctx, m.disk(), req.Sources, m.Dir, all...)
	if req.Command != "" {
		missing = unwritten(missing)
	}
	r.SetMissing(missing...)
	return r, nil
}

// unwritten is ms, the reports a command was to write that could not be
// read, as what they are: the command measured nothing.
func unwritten(ms []coverage.Unmeasured) []coverage.Unmeasured {
	out := slices.Clone(ms)
	for i := range out {
		if _, unreadable := out[i].Cause.(port.Unreadable); unreadable {
			out[i].Cause, out[i].Reason = port.MeasuredNothing{}, "--coverage-command wrote no readable report: "+out[i].Reason
		}
	}
	return out
}

// load is the entries of the report at path, read from Files in the
// format its content has, or why it could not be read: not there, as the
// disk's open says, or not a report.
func (m Measurer) load(path string) ([]coverage.Entry, error) {
	disk := m.disk()
	want := disk.abs(path)
	for name, text := range m.Files {
		if disk.abs(name) == want {
			return coverage.Parse(strings.NewReader(text))
		}
	}
	return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}

// canned is the reports of roots each the nearest of them to one of
// sources, merged in the roots' order.
func (m Measurer) canned(roots map[string]*coverage.Report, sources []string) *coverage.Report {
	disk := m.disk()
	nearest := map[string]bool{}
	for _, source := range sources {
		best, length := "", -1
		for root := range roots {
			dir := disk.abs(root)
			rel, err := filepath.Rel(dir, disk.abs(source))
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(dir) > length {
				best, length = root, len(dir)
			}
		}
		if best != "" {
			nearest[best] = true
		}
	}
	var reports []*coverage.Report
	for _, root := range slices.Sorted(maps.Keys(roots)) {
		if nearest[root] {
			reports = append(reports, roots[root])
		}
	}
	return coverage.Merge(reports...)
}

// disk is Files as the coverage port's Files fake holds them: every one a
// regular file.
func (m Measurer) disk() Files {
	return Files{Dir: m.Dir, Regular: slices.Collect(maps.Keys(m.Files))}
}
