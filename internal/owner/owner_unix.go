//go:build unix

package owner

import (
	"os"
	"strconv"
	"syscall"
)

func keep(dir, path string) {
	if os.Geteuid() != 0 {
		return
	}
	uid, gid, ok := of(dir)
	if !ok {
		return
	}
	if uid == 0 {
		// The directory is root's too: fall back to whoever asked for the
		// rights, which pkexec and sudo both report.
		uid, gid, ok = requester()
		if !ok {
			return
		}
	}
	_ = os.Chown(path, uid, gid)
}

// of reads the user a directory belongs to.
func of(dir string) (int, int, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}

// requester is the user an elevation helper acted for.
func requester() (int, int, bool) {
	for _, name := range []string{"PKEXEC_UID", "SUDO_UID"} {
		value := os.Getenv(name)
		if value == "" {
			continue
		}
		uid, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		gid := uid
		if value, ok := os.LookupEnv("SUDO_GID"); ok {
			if parsed, err := strconv.Atoi(value); err == nil {
				gid = parsed
			}
		}
		return uid, gid, true
	}
	return 0, 0, false
}
