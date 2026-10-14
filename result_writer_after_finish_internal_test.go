package evo

import (
	"bytes"
	"testing"
)

// A ResultWriter write after Finish is misuse in every Output mode: it returns an error, records the
// misuse, and never reaches stdout.
func TestResultWriterAfterFinishIsMisuseEveryMode(t *testing.T) {
	modes := map[string]Output{"human": Human, "plain": Plain, "json": JSON, "jsonl": JSONL}
	for name, mode := range modes {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := initHostRun(Config{Output: mode, stdout: &stdout, stderr: &stderr, color: colorNever})
			payload := out.ResultWriter()
			succeed(out.Task("work"))
			if err := out.finish(); err != nil {
				t.Fatal(err)
			}
			before := stdout.Len()

			n, err := payload.Write([]byte("late"))

			if err == nil || n != 0 {
				t.Fatalf("write after Finish = (%d, %v), want (0, error)", n, err)
			}
			if out.firstMisuse() == nil {
				t.Fatal("write after Finish did not record misuse")
			}
			if stdout.Len() != before {
				t.Fatalf("write after Finish reached stdout: %q", stdout.String()[before:])
			}
		})
	}
}
