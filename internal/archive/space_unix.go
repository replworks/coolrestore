//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package archive

import (
	"fmt"
	"syscall"
)

func availableBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
