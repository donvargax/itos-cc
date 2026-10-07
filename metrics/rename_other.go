//go:build !windows

package metrics

import "os"

// rename moves the staged snapshot onto its path. Off Windows a rename over
// an existing file is atomic and never fails because another one is in
// flight, so there is nothing to retry.
func rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
