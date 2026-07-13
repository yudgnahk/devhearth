//go:build darwin || linux

package detect

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, ErrSymlinkRefused
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
