package evo_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The v9b daemon panel's settled frames are the refused and stopped modes.
// Refused is Blocked, which the contract already has, so that frame is exact
// including the next step. Stopped is a successful Task whose summary says
// it stopped. The HTML's "[stopped]" band is not a run Conclusion (the
// contract's conclusion is only OK, Blocked, Failed, Cancelled), so the
// rendered band is "[ready]".
func TestUIV9b_DaemonRefusedIsBlocked(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true,
		Subject: "zqr controller",
		Stdout:  &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("controller")
	task.Block("already running (pid 73195, started 16:17)", evo.NextCommand("zqr", "controller", "stop"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"zqr controller",
		"blocked",
		"already running (pid 73195, started 16:17)",
		"zqr controller stop",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("daemon refused frame lacks %q\n%s", want, got)
		}
	}
	if !strings.Contains(got, "[blocked]") {
		t.Errorf("refused daemon must conclude [blocked]\n%s", got)
	}
}

func TestUIV9b_DaemonStoppedIsReady(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true,
		Subject: "zqr controller", Title: "zqr controller",
		Stdout: &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("controller").Summary("stopped after 4h12m").Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "stopped after 4h12m") {
		t.Fatalf("stopped frame lacks the summary\n%s", got)
	}
	if strings.Contains(got, "[stopped]") {
		t.Fatalf("stopped is not a conclusion band\n%s", got)
	}
	if !strings.Contains(got, "[ready]") {
		t.Fatalf("a stopped daemon that succeeded concludes [ready]\n%s", got)
	}
}
