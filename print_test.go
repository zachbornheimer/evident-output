package evo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

func TestPrint_MatchesFmtConstruction(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	out.printf("Found %d packages\n", 18)
	out.println("done")
	_ = out.finish()
	s := buf.String()
	if !strings.Contains(s, "Found 18 packages") || !strings.Contains(s, "done") {
		t.Fatalf("%s", s)
	}
	snap := out.snapshot()
	if len(snap.Messages) < 2 {
		t.Fatalf("messages: %+v", snap.Messages)
	}
}

func TestPrint_FragmentsCombine(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	out.print("down")
	out.print("loading")
	out.print("...\n")
	_ = out.finish()
	if !strings.Contains(buf.String(), "downloading...") {
		t.Fatalf("%s", buf.String())
	}
	if len(out.snapshot().Messages) != 1 {
		t.Fatalf("want 1 message, got %+v", out.snapshot().Messages)
	}
}

func TestPrint_TrailingFragmentFlushedAtFinish(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	out.print("partial-no-nl")
	_ = out.finish()
	if !strings.Contains(buf.String(), "partial-no-nl") {
		t.Fatalf("%s", buf.String())
	}
}

func TestVerbose_HiddenAtNormal(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	out.println("visible")
	out.atForTest(visibilityVerbose).println("hidden detail")
	_ = out.finish()
	if !strings.Contains(buf.String(), "visible") {
		t.Fatal("normal missing")
	}
	if strings.Contains(buf.String(), "hidden detail") {
		t.Fatalf("verbose leaked:\n%s", buf.String())
	}
	// Canonical model still holds verbose message.
	var found bool
	for _, m := range out.snapshot().Messages {
		if m.Visibility == visibilityVerbose && strings.Contains(m.Text, "hidden detail") {
			found = true
		}
	}
	if !found {
		t.Fatalf("verbose message missing from snapshot: %+v", out.snapshot().Messages)
	}
}

func TestVerbose_ShownWhenConfigured(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{
		stdout:    &buf,
		stderr:    &buf,
		verbosity: verbosityVerbose,
	})
	out.atForTest(visibilityVerbose).printf("Cache: %s\n", "/tmp/x")
	_ = out.finish()
	if !strings.Contains(buf.String(), "Cache: /tmp/x") {
		t.Fatalf("%s", buf.String())
	}
}

func TestPrint_CRLFAndSanitize(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	out.print("a\r\nb\x1b[31mx\n")
	_ = out.finish()
	s := buf.String()
	if strings.Contains(s, "\r") {
		t.Fatalf("CR remained: %q", s)
	}
	if strings.Contains(s, "\x1b") {
		t.Fatalf("ESC leaked: %q", s)
	}
}

func TestPrint_WriterAdapter(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	w := out.writer()
	_, _ = w.Write([]byte("from-writer\n"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "from-writer") {
		t.Fatal(buf.String())
	}
}

// TestTask_PrintfNames is C6: Task is printf-variadic itself (Taskf
// deleted; Item folded into Task).
func TestTask_PrintfNames(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, stderr: &buf})
	succeed(out.Task(fmt.Sprintf("repo %s", "x")))
	succeed(out.Task(fmt.Sprintf("check %d", 1)), "ok")
	_ = out.finish()
	if !strings.Contains(buf.String(), "repo x") || !strings.Contains(buf.String(), "check 1") {
		t.Fatal(buf.String())
	}
}

// TestEncodeJSON_ContainsTasks is C8: WriteJSON (an io.Writer wrapper adding
// a trailing newline) is deleted — EncodeJSON is the surviving encoder; a
// caller who wants the newline appends it themselves.
func TestEncodeJSON_ContainsTasks(t *testing.T) {
	out := initRun(Config{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}})
	succeed(out.Task("a"))
	_ = out.finish()
	b, err := wire.EncodeJSON(out.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatalf("json: %s", b)
	}
}

func TestAbout_DeclareDryRun_MergesRepoPath(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })
	out.aboutForTest("repo  /Users/zbornheimer/Developer/Zysys/flight")
	out.declareDryRunForTest()
	got := buf.String()
	if !strings.Contains(got, "repo  /Users/zbornheimer/Developer/Zysys/flight") {
		t.Fatalf("dry-run header missing repo path: %q", got)
	}
}
