package domain

import (
	"context"
	"errors"

	"github.com/donvargax/itos-cc/internal/lang"
)

// Rehashable says whether e, an exception of f, whose sites are sites, was
// written under itos-cc's previous hash and its function changed only in
// what the hash now leaves out: its function, by namespace#name, hashed by
// PreviousUnitHash, has e's hash, and a site of e's change is at e's place
// in it. It returns the function's hash now and the site's line.
func Rehashable(e Exception, f *lang.File, sites []Site) (hash string, line int, ok bool) {
	for i, u := range f.Units {
		if UnitID(u.Namespace, u.Name) != e.Function || PreviousUnitHash(f, u) != e.Hash {
			continue
		}
		at := u.StartLine + e.LineInFunction - 1
		for _, s := range sites {
			if s.Unit == i && s.Line == at && s.Column == e.Column && s.Original == e.Original && s.Replacement == e.Replacement {
				return UnitHash(f, u), s.Line, true
			}
		}
	}
	return "", 0, false
}

// Rehash judges each exception of exceptions that no longer holds, reading
// only the file it names, from the project root: one Rehashable there is
// carried over, the renewal's New being the entry with its function's hash
// now and nothing else changed, and Line the site's line; any other is
// left, stale as Locate says, or ExceptionGone, with no line, when its file
// is no source. It runs nothing, reads no snapshot, and writes nothing;
// the caller writes the renewals through Exceptions.Rehash.
func Rehash(ctx context.Context, exceptions []Exception, opt CheckOptions, status SourceStatus) ([]Renewal, []StaleException, error) {
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if len(exceptions) == 0 {
		return nil, nil, nil
	}
	if status == nil {
		return nil, nil, errors.New("mutation source status binding is missing")
	}
	type parsed struct {
		file  *lang.File
		sites []Site
	}
	files := map[string]parsed{}
	defer func() {
		for _, f := range files {
			f.file.Close()
		}
	}()
	var rehashed []Renewal
	var left []StaleException
	for _, e := range exceptions {
		path := absoluteSource(opt.Root, e.File)
		f, ok := files[path]
		if !ok {
			kind, failure := status.Kind(ctx, path)
			if failure != nil {
				return nil, nil, failure
			}
			if kind != SourceNonDirectory || lang.Detect(path) == nil {
				left = append(left, StaleException{Exception: e, Why: ExceptionGone})
				continue
			}
			file, err := parseExceptionSource(ctx, path, opt)
			if err != nil {
				return nil, nil, err
			}
			f = parsed{file: file, sites: Sites(file)}
			files[path] = f
		}
		_, why, line := Locate(e, f.file, f.sites)
		if why == "" {
			continue
		}
		if hash, at, ok := Rehashable(e, f.file, f.sites); ok {
			renewed := e
			renewed.Hash = hash
			rehashed = append(rehashed, Renewal{Old: e, New: renewed, Line: at})
			continue
		}
		left = append(left, StaleException{Exception: e, Why: why, Line: line})
	}
	return rehashed, left, stopped(ctx, nil)
}
