//go:build linux || darwin

package securefile

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openRegularWithin(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	err = unix.Fstat(int(file.Fd()), &stat)
	if err == nil && (stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1) {
		err = fmt.Errorf("policy path must be a single-link regular file")
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
