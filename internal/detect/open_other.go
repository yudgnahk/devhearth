//go:build !darwin && !linux

package detect

import (
	"os"
)

// openNoFollow refuses symlink paths via Lstat before opening. This is a
// best-effort guard on platforms without O_NOFOLLOW in our build tags.
func openNoFollow(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlinkRefused
	}
	return os.Open(path)
}
