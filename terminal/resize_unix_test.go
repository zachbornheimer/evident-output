//go:build unix

package terminal_test

import (
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/terminal"
)

func TestANSI_StartResizeWatch_InvokesCallbackOnSIGWINCH(t *testing.T) {
	f := os.Stdout
	drv := terminal.NewANSI(f,
		terminal.WithInteractive(true),
		terminal.WithSize(80, 24),
		terminal.WithSizeFile(f),
	)

	var n atomic.Int32
	drv.StartResizeWatch(func() { n.Add(1) })
	t.Cleanup(drv.StopResizeWatch)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected resize callback after SIGWINCH")
}

func TestANSI_StopResizeWatch_Idempotent(t *testing.T) {
	drv := terminal.NewANSI(os.Stdout, terminal.WithSizeFile(os.Stdout))
	drv.StartResizeWatch(func() {})
	drv.StopResizeWatch()
	drv.StopResizeWatch()
}
