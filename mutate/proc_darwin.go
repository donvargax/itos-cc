//go:build darwin

package mutate

import (
	"errors"
	"syscall"
)

// groupKillDeniedForZombies is true: macOS refuses kill(-pgid, sig) with
// EPERM when every member left in the group is a zombie, as after the kill
// a context cancellation already sent, so the group kill tolerates it and
// leaves the verdict to ownedGroupGone.
const groupKillDeniedForZombies = true

// ownedGroupGone reports whether the killed process group pgid has no member
// left. macOS has no /proc: the group is gone once kill(-pgid, 0) reports
// ESRCH. Its members' parents die in the same kill, so launchd inherits and
// reaps their zombies, and none holds the group past the cleanup deadline.
// EPERM still means a member exists, a zombie launchd has not reaped yet
// or one that refuses the signal, so the wait goes on to its deadline.
func ownedGroupGone(pgid int) (bool, error) {
	err := syscall.Kill(-pgid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false, err
	}
	return false, nil
}
