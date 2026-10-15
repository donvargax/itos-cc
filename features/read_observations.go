package features

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"time"
)

// ErrReadObservationUnsupported distinguishes missing observation from failed I/O.
var ErrReadObservationUnsupported = errors.New("read observation unsupported")

// ObserveReads proves observation on every owned target before marking it. It
// changes access times only, preserving modification times and fixture contents.
func (binary) ObserveReads(paths ...string) (func() ([]string, error), error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("read observation needs owned targets")
	}
	paths = slices.Clone(paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	marks := map[string]time.Time{}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("read observation target is not a regular file or directory: %s", path)
		}
		if _, ok := accessedAt(info); !ok {
			return nil, fmt.Errorf("%w: access time unavailable for %s", ErrReadObservationUnsupported, path)
		}
		past := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, past, info.ModTime()); err != nil {
			return nil, err
		}
		marked, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		mark, ok := accessedAt(marked)
		if !ok {
			return nil, fmt.Errorf("%w: access time unavailable for %s", ErrReadObservationUnsupported, path)
		}
		if info.IsDir() {
			_, err = os.ReadDir(path)
		} else {
			_, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, err
		}
		read, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		at, ok := accessedAt(read)
		if !ok || !at.After(mark) {
			return nil, fmt.Errorf("%w: positive read/list did not update %s", ErrReadObservationUnsupported, path)
		}
		if err := os.Chtimes(path, mark, info.ModTime()); err != nil {
			return nil, err
		}
		marks[path] = mark
	}
	return func() ([]string, error) {
		var read []string
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			at, ok := accessedAt(info)
			if !ok {
				return nil, fmt.Errorf("%w: access time unavailable for %s", ErrReadObservationUnsupported, path)
			}
			if at.After(marks[path]) {
				read = append(read, path)
			}
		}
		return read, nil
	}, nil
}

// accessedAt reads the platform's metadata representation without selecting the
// host OS in logic. A platform without either supported field is explicit.
func accessedAt(info os.FileInfo) (time.Time, bool) {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return time.Time{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return time.Time{}, false
	}
	for _, name := range []string{"Atim", "Atimespec"} {
		f := v.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.Struct {
			sec, nsec := f.FieldByName("Sec"), f.FieldByName("Nsec")
			if sec.IsValid() && nsec.IsValid() && sec.CanInt() && nsec.CanInt() {
				return time.Unix(sec.Int(), nsec.Int()), true
			}
		}
	}
	return time.Time{}, false
}
