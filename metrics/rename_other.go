//go:build !windows

package metrics

// ephemeral reports whether err is one the file operations get only for a
// moment: never, off Windows, where a rename over an existing file is
// atomic and never fails because another one or a reader is in flight.
var ephemeral = func(error) bool { return false }
