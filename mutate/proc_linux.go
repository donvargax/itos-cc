//go:build linux

package mutate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ownedGroupGone reports whether the killed process group pgid has no member
// left that can run. Linux counts zombies as gone by scanning /proc, since a
// zombie whose parent does not reap it keeps kill(-pgid, 0) succeeding.
func ownedGroupGone(pgid int) (bool, error) {
	err := syscall.Kill(-pgid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false, err
	}
	running, err := ownedGroupHasRunnableProcess(pgid)
	if err != nil {
		return false, err
	}
	return !running, nil
}

func ownedGroupHasRunnableProcess(pgid int) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
				continue
			}
			return false, err
		}
		closeParen := strings.LastIndexByte(string(data), ')')
		if closeParen < 0 {
			return false, fmt.Errorf("malformed process stat for pid %s", entry.Name())
		}
		fields := strings.Fields(string(data[closeParen+1:]))
		if len(fields) < 3 {
			return false, fmt.Errorf("short process stat for pid %s", entry.Name())
		}
		group, err := strconv.Atoi(fields[2])
		if err != nil {
			return false, fmt.Errorf("parse process group for pid %s: %w", entry.Name(), err)
		}
		if group == pgid && fields[0] != "Z" && fields[0] != "X" {
			return true, nil
		}
	}
	return false, nil
}
