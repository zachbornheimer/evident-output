package evo

import (
	"os"
	"testing"
)

// openCharDevice opens path twice, standing in for two distinct fds (e.g.
// Stdout fd 1 and Stderr fd 2) that a shell attached to one physical
// terminal without redirection. /dev/null is used instead of a real tty
// because CI and sandboxed shells commonly have no controlling terminal
// (/dev/tty) to open, but it is a genuine character device with a fixed
// device/inode — exactly the shape os.SameFile and sameTerminalDevice
// compare — so it exercises the identity logic portably.
func openCharDevice(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	a, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no character device available to open: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no character device available to open: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return a, b
}

// TestSameTerminalDevice_TwoFilesOnOneCharDevice proves sameTerminalDevice
// answers true for two distinct *os.File values that name the same physical
// character device (the Stdout-fd1/Stderr-fd2-share-one-tty shape from
// gate-7 finding 1), and false when either side is not a character device
// or the two are genuinely different devices.
func TestSameTerminalDevice_TwoFilesOnOneCharDevice(t *testing.T) {
	a, b := openCharDevice(t)
	if !sameTerminalDevice(a, b) {
		t.Fatal("expected two fds on the same character device to compare equal")
	}

	tmp, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tmp.Close() })
	if sameTerminalDevice(a, tmp) {
		t.Fatal("expected a character device and a regular file to never match")
	}

	var notFile writerStub
	if sameTerminalDevice(a, &notFile) {
		t.Fatal("expected a non-*os.File writer to never match")
	}
}

type writerStub struct{}

func (writerStub) Write(p []byte) (int, error) { return len(p), nil }

// TestConfigToOptions_DiagnosticSharingTerminalIsDetected proves configToOptions
// sets diagnosticSharesTerminal when Config.Stdout (the live terminal's
// writer) and Config.Stderr (Diagnostics) resolve to the same physical
// device, even though they are distinct io.Writer values — the realistic
// default construction (To(Stdout), Diagnostics(Stderr)) on an interactive
// shell without redirection.
func TestConfigToOptions_DiagnosticSharingTerminalIsDetected(t *testing.T) {
	stdout, stderr := openCharDevice(t)

	cfg := resolveConfig(Config{Stdout: stdout, Stderr: stderr})
	opts := configToOptions(cfg)

	var built config
	for _, o := range opts {
		o.apply(&built)
	}
	if !built.diagnosticSharesTerminal {
		t.Fatal("expected diagnosticSharesTerminal to be detected when Stdout and Stderr share one device")
	}
}
