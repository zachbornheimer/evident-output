//go:build darwin || linux

package evo

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const (
	ptyRows        = 24
	ptyCols        = 80
	ptyDrainBudget = 5 * time.Second
)

// ptyScreen is a pseudo-terminal a program writes to through two separate file descriptors, as a shell
// attaches fd 1 and fd 2 to one tty, and a test reads back as the screen the user would see.
type ptyScreen struct {
	// Stdout and Stderr are two opens of the one slave device.
	Stdout, Stderr *os.File

	master  *os.File
	drained chan struct{}
	raw     bytes.Buffer
}

func newPtyScreen(t *testing.T) *ptyScreen {
	t.Helper()
	master, slavePath, err := openPtyPair()
	if err != nil {
		t.Skipf("no pseudo-terminal available: %v", err)
	}
	p := &ptyScreen{master: master, drained: make(chan struct{})}
	if p.Stdout, err = os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0); err != nil {
		t.Fatalf("open pty slave for stdout: %v", err)
	}
	if p.Stderr, err = os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0); err != nil {
		t.Fatalf("open pty slave for stderr: %v", err)
	}
	if err := unix.IoctlSetWinsize(int(p.Stdout.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: ptyRows, Col: ptyCols}); err != nil {
		t.Fatalf("size pty: %v", err)
	}
	go func() {
		defer close(p.drained)
		_, _ = io.Copy(&p.raw, master)
	}()
	t.Cleanup(func() { _ = p.Stdout.Close(); _ = p.Stderr.Close(); _ = master.Close() })
	return p
}

// Lines closes the slave, waits for every byte the program wrote to arrive, and returns the screen rows
// after replaying those bytes through a terminal.
func (p *ptyScreen) Lines(t *testing.T) []string {
	t.Helper()
	_ = p.Stdout.Close()
	_ = p.Stderr.Close()
	select {
	case <-p.drained:
	case <-time.After(ptyDrainBudget):
		t.Fatal("pty output did not drain after the slave closed")
	}
	return replayTerminal(p.raw.Bytes())
}
