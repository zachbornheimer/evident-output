package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Verdict is what one named test did in a `go test -json` run.
type Verdict string

const (
	// Red means the test failed, panicked, or was killed by the timeout.
	Red Verdict = "red"
	// Green means the test passed.
	Green Verdict = "green"
	// Skipped means the test reported a skip, which proves nothing.
	Skipped Verdict = "skipped"
	// NotRun means no event named the test: a build failure or a wrong name.
	NotRun Verdict = "not-run"
)

const outputTailBytes = 1200

type testEvent struct {
	Action string
	Test   string
	Output string
}

// Verdicts classifies each name from `go test -json` output. A test that
// started and never reported a result was killed mid-run and counts as red.
func Verdicts(out []byte, names []string) map[string]Verdict {
	started, last := map[string]bool{}, map[string]string{}
	for _, ev := range testEvents(out) {
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "run":
			started[ev.Test] = true
		case "pass", "fail", "skip":
			last[ev.Test] = ev.Action
		}
	}
	verdicts := make(map[string]Verdict, len(names))
	for _, name := range names {
		verdicts[name] = verdictOf(started[name], last[name])
	}
	return verdicts
}

func verdictOf(started bool, result string) Verdict {
	switch {
	case result == "fail":
		return Red
	case result == "pass":
		return Green
	case result == "skip":
		return Skipped
	case started:
		return Red
	}
	return NotRun
}

// PackageOutput is the tail of the output not tied to one test, where a build
// failure or a timeout message lands.
func PackageOutput(out []byte) string {
	var text strings.Builder
	for _, ev := range testEvents(out) {
		if ev.Test == "" {
			text.WriteString(ev.Output)
		}
	}
	tail := strings.TrimSpace(text.String())
	if len(tail) > outputTailBytes {
		tail = "..." + tail[len(tail)-outputTailBytes:]
	}
	return tail
}

func testEvents(out []byte) []testEvent {
	var events []testEvent
	for line := range bytes.SplitSeq(out, []byte("\n")) {
		var ev testEvent
		if json.Unmarshal(line, &ev) == nil && ev.Action != "" {
			events = append(events, ev)
		}
	}
	return events
}
