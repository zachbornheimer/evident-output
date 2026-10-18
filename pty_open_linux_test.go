//go:build linux

package evo

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const ptmxPath = "/dev/ptmx"

// openPtyPair opens a pseudo-terminal: the master end a test reads the screen bytes from, and the slave path
// a program's stdout and stderr open as one character device.
func openPtyPair() (master *os.File, slavePath string, err error) {
	master, err = os.OpenFile(ptmxPath, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	fd := int(master.Fd())
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, "", err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, "", err
	}
	return master, fmt.Sprintf("/dev/pts/%d", n), nil
}
