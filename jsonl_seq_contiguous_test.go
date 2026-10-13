package evo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestJSONLSeqIsContiguous pins that the evo.event stream numbers its own
// lines 1, 2, 3 ...: a consumer detects a lost line by a gap, so the run's
// journal-only events (debug records, task states no line announces) must
// not consume stream numbers.
func TestJSONLSeqIsContiguous(t *testing.T) {
	var stream, human bytes.Buffer
	o := newOutput("job", to(&human), maxConcurrency(2), wireFormatOption(formatJSONL), wireStreamOption(&stream))
	o.debug("journal-only record")
	for _, name := range []string{"a", "b", "c"} {
		o.Task(name).succeed("")
		o.debug("between tasks")
	}
	if err := o.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stream.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("evo.event stream has %d lines, want several:\n%s", len(lines), stream.String())
	}
	for i, line := range lines {
		var event struct {
			Seq uint64 `json:"seq"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d is not JSON: %v", i+1, err)
		}
		if want := uint64(i + 1); event.Seq != want {
			t.Fatalf("line %d has seq %d, want %d (contiguous per stream):\n%s", i+1, event.Seq, want, stream.String())
		}
	}
}
