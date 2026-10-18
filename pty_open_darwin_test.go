//go:build darwin

package evo

import (
	"os"
	"syscall"
	"unsafe"

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
	if err = unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		_ = master.Close()
		return nil, "", err
	}
	if err = unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		_ = master.Close()
		return nil, "", err
	}
	var name [128]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		_ = master.Close()
		return nil, "", errno
	}
	return master, unix.ByteSliceToString(name[:]), nil
}
