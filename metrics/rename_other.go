//go:build !windows

package metrics

// rename moves the staged snapshot onto its path. Off Windows a rename over
// an existing file is atomic and never fails because another one is in
// flight, so there is nothing to retry.
func rename(oldpath, newpath string) error {
	return osRename(oldpath, newpath)
}

// ephemeral reports whether err is one the file operations get only for a
// moment: never, off Windows.
var ephemeral = func(error) bool { return false }
