package evo

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// TestRunIDIsUniquePerProcess pins that two runs never share a run_id, even on one frozen Clock, and that
// the evo.run document and the evo.event stream of one run carry the same id.
func TestRunIDIsUniquePerProcess(t *testing.T) {
	frozen := TestClock{T: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	runOnce := func() (document, event string) {
		var stdout bytes.Buffer
		out := newOutput("job", to(io.Discard), withClock(frozen), wireFormatOption(JSONL), wireStreamOption(&stdout))
		out.Task("a").succeed("")
		if err := out.finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}
		first, _, _ := strings.Cut(stdout.String(), "\n")
		var line struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(first), &line); err != nil {
			t.Fatalf("first evo.event line is not JSON: %v\n%s", err, stdout.String())
		}
		return out.decodedRunDocument(t).RunID, line.RunID
	}

	doc1, event1 := runOnce()
	doc2, event2 := runOnce()
	if doc1 == "" || event1 == "" {
		t.Fatalf("run_id is empty: evo.run %q, evo.event %q", doc1, event1)
	}
	if doc1 != event1 || doc2 != event2 {
		t.Errorf("one run carries two ids: evo.run %q vs evo.event %q, then %q vs %q", doc1, event1, doc2, event2)
	}
	if doc1 == doc2 {
		t.Errorf("two runs share run_id %q", doc1)
	}
}
