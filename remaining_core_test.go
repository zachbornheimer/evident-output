package evo_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestLOG014_WarnMessageDistinctFromItemWarn(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Println("log warning")
	out.Task("i").Problem("item warning", evo.Severity(evo.SeverityWarning))
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "log warning") || !strings.Contains(s, "item warning") {
		t.Fatal(s)
	}
}

func TestLOG008_ConcurrentDebugWriters(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := out.DebugWriterForTest()
			_, _ = w.Write([]byte("line\n"))
			_ = w.Close()
		}(i)
	}
	wg.Wait()
	_ = out.Finish()
}
