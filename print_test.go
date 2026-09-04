package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestPrint_MatchesFmtConstruction(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Printf("Found %d packages\n", 18)
	out.Println("done")
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "Found 18 packages") || !strings.Contains(s, "done") {
		t.Fatalf("%s", s)
	}
	snap := out.Snapshot()
	if len(snap.Messages) < 2 {
		t.Fatalf("messages: %+v", snap.Messages)
	}
}

func TestPrint_FragmentsCombine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Print("down")
	out.Print("loading")
	out.Print("...\n")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "downloading...") {
		t.Fatalf("%s", buf.String())
	}
	if len(out.Snapshot().Messages) != 1 {
		t.Fatalf("want 1 message, got %+v", out.Snapshot().Messages)
	}
}

func TestPrint_TrailingFragmentFlushedAtFinish(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Print("partial-no-nl")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "partial-no-nl") {
		t.Fatalf("%s", buf.String())
	}
}

func TestVerbose_HiddenAtNormal(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Println("visible")
	out.At(evo.VisibilityVerbose).Println("hidden detail")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "visible") {
		t.Fatal("normal missing")
	}
	if strings.Contains(buf.String(), "hidden detail") {
		t.Fatalf("verbose leaked:\n%s", buf.String())
	}
	// Canonical model still holds verbose message.
	var found bool
	for _, m := range out.Snapshot().Messages {
		if m.Visibility == evo.VisibilityVerbose && strings.Contains(m.Text, "hidden detail") {
			found = true
		}
	}
	if !found {
		t.Fatalf("verbose message missing from snapshot: %+v", out.Snapshot().Messages)
	}
}

func TestVerbose_ShownWhenConfigured(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Stdout:    &buf,
		Stderr:    &buf,
		Verbosity: evo.VerbosityVerbose,
	})
	out.At(evo.VisibilityVerbose).Printf("Cache: %s\n", "/tmp/x")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "Cache: /tmp/x") {
		t.Fatalf("%s", buf.String())
	}
}

func TestPrint_CRLFAndSanitize(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Print("a\r\nb\x1b[31mx\n")
	_ = out.Finish()
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
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	w := out.Writer()
	_, _ = w.Write([]byte("from-writer\n"))
	_ = out.Finish()
	if !strings.Contains(buf.String(), "from-writer") {
		t.Fatal(buf.String())
	}
}

// TestItem_Task_PrintfNames is C6: Item/Task are printf-variadic
// themselves (Itemf/Taskf deleted).
func TestItem_Task_PrintfNames(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf})
	out.Item("repo %s", "x").Done()
	out.Task("check %d", 1).Done("ok")
	_ = out.Finish()
	if !strings.Contains(buf.String(), "repo x") || !strings.Contains(buf.String(), "check 1") {
		t.Fatal(buf.String())
	}
}

// TestEncodeJSON_ContainsTasks is C8: WriteJSON (an io.Writer wrapper adding
// a trailing newline) is deleted — EncodeJSON is the surviving encoder; a
// caller who wants the newline appends it themselves.
func TestEncodeJSON_ContainsTasks(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	out.Task("a").Done()
	_ = out.Finish()
	b, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatalf("json: %s", b)
	}
}
