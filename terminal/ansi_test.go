package terminal_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
)

func TestANSI_LiveRegionUsesCursorControl(t *testing.T) {
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithSize(80, 24), terminal.WithInteractive(true))

	out := evo.New(evo.Terminal(drv), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("work")
	task.Phase("running")
	out.Debug("note")
	task.Donef("done")
	_ = out.Finish()

	s := buf.String()
	// Must hide cursor, erase/move for live region, and restore cursor on final.
	if !strings.Contains(s, "\x1b[?25l") {
		t.Fatalf("expected hide cursor in output: %q", s)
	}
	if !strings.Contains(s, "\x1b[?25h") {
		t.Fatalf("expected show cursor in output: %q", s)
	}
	if !strings.Contains(s, "\x1b[2K") {
		t.Fatalf("expected erase line in live updates: %q", s)
	}
	if !strings.Contains(s, "done") {
		t.Fatalf("expected final text: %q", s)
	}
}

func TestANSI_ClearLiveErasesPreviousFrame(t *testing.T) {
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithInteractive(true), terminal.WithSize(40, 10))
	drv.WriteLive("line1\nline2")
	if drv.LiveLines() != 2 {
		t.Fatalf("live lines=%d", drv.LiveLines())
	}
	drv.ClearLive()
	if drv.LiveLines() != 0 {
		t.Fatal("live lines should be 0 after clear")
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("expected ANSI after clear")
	}
}

func TestANSI_NonInteractiveWritesPlain(t *testing.T) {
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithInteractive(false))
	drv.WriteFinal("hello")
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("non-interactive should not emit CSI: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatal(buf.String())
	}
}
