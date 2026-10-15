package features

import (
	"os"
	"time"
)

// ObserveWrites dates existing files well in the past, making even a rewrite
// of identical bytes observable without sleeping for a filesystem clock tick.
// The returned observer reports paths written or removed since it was made.
func (binary) ObserveWrites(paths ...string) (func() ([]string, error), error) {
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, path := range paths {
		if err := os.Chtimes(path, past, past); err != nil {
			return nil, err
		}
	}
	return func() ([]string, error) {
		var written []string
		for _, path := range paths {
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				written = append(written, path)
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.ModTime().Equal(past) {
				written = append(written, path)
			}
		}
		return written, nil
	}, nil
}
