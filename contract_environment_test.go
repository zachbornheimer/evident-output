package evo_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// contractEnvironment is every variable CONTRACT.md §6 lists, plus the ones
// it does not list that Evo has been seen to read. Each case starts from all
// of them unset.
var contractEnvironment = []string{"EVO_OUTPUT", "EVO_VERBOSE", "EVO_DEBUG", "NO_COLOR", "FORCE_COLOR", "EVO_COLOR"}

// ansiEscape starts every color sequence.
const ansiEscape = "\x1b["

// withEnvironment unsets every contract variable, then sets vars.
func withEnvironment(t *testing.T, vars map[string]string) {
	t.Helper()
	for _, name := range contractEnvironment {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
	for name, value := range vars {
		t.Setenv(name, value)
	}
}

// isRunDocument reports whether stdout is one evo.run document.
func isRunDocument(stdout string) bool {
	var doc map[string]any
	return json.Unmarshal([]byte(stdout), &doc) == nil && doc["object"] == "evo.run"
}

// isEventStream reports whether every stdout line is an evo.event.
func isEventStream(stdout string) bool {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for _, line := range lines {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil || event["object"] != "evo.event" {
			return false
		}
	}
	return len(lines) > 0
}

// TestEnvironmentMatchesContract pins CONTRACT.md §6: the environment table
// is the whole environment surface, applied only where Config is zero, and
// a value outside its list does nothing.
func TestEnvironmentMatchesContract(t *testing.T) {
	t.Run("EVO_OUTPUT", testEvoOutput)
	t.Run("EVO_VERBOSE", testEvoVerbose)
	t.Run("EVO_DEBUG", testEvoDebug)
	t.Run("color", testColorEnvironment)
}

func testEvoOutput(t *testing.T) {
	cases := []struct {
		value   string
		cfg     evo.Output
		machine string // "json", "jsonl" or "" for text
	}{
		{"json", 0, "json"},
		{"jsonl", 0, "jsonl"},
		{"plain", 0, ""},
		{"human", 0, ""},
		{"stream-json", 0, ""},   // not in the list: ignored
		{"jsonl", evo.Plain, ""}, // Config wins over the environment
		{"", 0, ""},              // a piped stdout never infers machine output
	}
	for _, tc := range cases {
		t.Run(tc.value+"/config="+outputName[tc.cfg], func(t *testing.T) {
			withEnvironment(t, map[string]string{"EVO_OUTPUT": tc.value})
			run := executeContract(context.Background(), t, evo.Config{Output: tc.cfg}, scanWithFact)
			gotDoc, gotEvents := isRunDocument(run.Stdout), isEventStream(run.Stdout)
			if gotDoc != (tc.machine == "json") || gotEvents != (tc.machine == "jsonl") {
				t.Errorf("stdout is evo.run=%v evo.event=%v, want machine output %q:\n%s",
					gotDoc, gotEvents, tc.machine, run.Stdout)
			}
		})
	}
}

func testEvoVerbose(t *testing.T) {
	withEnvironment(t, map[string]string{"EVO_VERBOSE": "1"})
	run := executeContract(context.Background(), t, evo.Config{Output: evo.Plain}, scanWithFact)
	if !strings.Contains(run.Text(), configFactText) {
		t.Errorf("EVO_VERBOSE=1 does not show the Fact:\n%s", run.Text())
	}
}

// testEvoDebug reads the journal's level through Run.SlogHandler: the level
// EVO_DEBUG names is enabled and the one below it is not.
func testEvoDebug(t *testing.T) {
	cases := []struct {
		value string
		level slog.Level
		valid bool
	}{
		{"debug", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"trace", slog.LevelDebug, false}, // not in the list: ignored
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			withEnvironment(t, map[string]string{"EVO_DEBUG": tc.value})
			var at, below bool
			executeContract(context.Background(), t, evo.Config{Output: evo.Plain},
				func(ctx context.Context, r *evo.Run) error {
					h := r.SlogHandler()
					at, below = h.Enabled(ctx, tc.level), h.Enabled(ctx, tc.level-1)
					return nil
				})
			if tc.valid && (!at || below) {
				t.Errorf("EVO_DEBUG=%s: journal enabled at %s=%v and below it=%v, want true and false",
					tc.value, tc.level, at, below)
			}
			if !tc.valid && at {
				t.Errorf("EVO_DEBUG=%s is not a contract level, yet the journal is enabled at %s", tc.value, tc.level)
			}
		})
	}
}

func testColorEnvironment(t *testing.T) {
	cases := []struct {
		name  string
		vars  map[string]string
		color bool
	}{
		{"FORCE_COLOR forces color off a terminal", map[string]string{"FORCE_COLOR": "1"}, true},
		{"EVO_COLOR is not read", map[string]string{"EVO_COLOR": "always"}, false},
		{"NO_COLOR keeps color off", map[string]string{"NO_COLOR": "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withEnvironment(t, tc.vars)
			run := executeContract(context.Background(), t, evo.Config{Output: evo.Human}, scanWithFact)
			if got := strings.Contains(run.Text(), ansiEscape); got != tc.color {
				t.Errorf("Human output has color = %v, want %v:\n%q", got, tc.color, run.Text())
			}
		})
	}
}
