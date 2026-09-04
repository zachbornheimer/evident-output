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

	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv), evo.VisibilityDelay(0), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("work")
	task.Doing("running")
	out.Debug("note")
	task.Done("done")
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

func TestANSI_WriteLiveRedrawsInPlace(t *testing.T) {
	// Regression: trailing newline after a frame left the cursor one row below
	// the live region, so the next erase missed prior content and frames scrolled.
	var buf bytes.Buffer
	drv := terminal.NewANSI(&buf, terminal.WithInteractive(true), terminal.WithSize(40, 10))
	drv.WriteLive("frame-a")
	if drv.LiveLines() != 1 {
		t.Fatalf("liveLines=%d", drv.LiveLines())
	}
	drv.WriteLive("frame-b")
	s := buf.String()
	// Second paint must erase (CSI erase line) before writing frame-b.
	if !strings.Contains(s, "\x1b[2K") {
		t.Fatalf("expected erase between frames: %q", s)
	}
	// Visible text after stripping CSI: last occurrence should be frame-b alone
	// as the terminal would show after erase+rewrite.
	stripped := stripCSI(s)
	// frame-a is painted once then erased from the cell (erase is CSI, so
	// frame-a remains in the byte stream once; frame-b once after).
	if strings.Count(stripped, "frame-a") != 1 || strings.Count(stripped, "frame-b") != 1 {
		t.Fatalf("unexpected frame dumps: %q", stripped)
	}
	// Multi-line: 3-line frame then smaller frame must CUU.
	buf.Reset()
	drv2 := terminal.NewANSI(&buf, terminal.WithInteractive(true), terminal.WithSize(40, 10))
	drv2.WriteLive("l1\nl2\nl3")
	if drv2.LiveLines() != 3 {
		t.Fatalf("liveLines=%d", drv2.LiveLines())
	}
	drv2.WriteLive("only")
	if !strings.Contains(buf.String(), "\x1b[2A") { // up 2 from last line to first
		t.Fatalf("expected CUU for 3-line erase: %q", buf.String())
	}
}

func stripCSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) {
			// skip CSI
			if s[i+1] == '[' {
				i += 2
				for i < len(s) {
					c := s[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				continue
			}
			i += 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
