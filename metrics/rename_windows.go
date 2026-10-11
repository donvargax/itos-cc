//go:build windows

package metrics

import (
	"errors"
	"syscall"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION, which package syscall
// does not name.
const errorSharingViolation syscall.Errno = 32

// ephemeral reports whether err is one Windows returns while another
// process or goroutine holds a snapshot for a moment: ERROR_ACCESS_DENIED,
// which a rename onto a file another rename is replacing or a reader has
// open gets, and an open of a file being replaced may get, or
// ERROR_SHARING_VIOLATION. ERROR_FILE_NOT_FOUND is not one: a snapshot that
// is not there is common, and a rename replaces its target in one step.
var ephemeral = func(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.ERROR_ACCESS_DENIED || errno == errorSharingViolation
}
