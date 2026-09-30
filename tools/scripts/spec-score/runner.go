package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const (
	actionPass = "pass"
	actionFail = "fail"
	actionSkip = "skip"
)

// TestRunner runs the named tests of one package and returns the raw
// `go test -json` event stream. Tests substitute a fake.
type TestRunner interface {
	Run(root, pkg, tags string, names []string) ([]byte, error)
}

type goTestRunner struct{}

func runPattern(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

// Run executes go test. A failing test run exits non-zero yet still emits a
// usable event stream, so an exit error only counts when nothing was emitted.
func (goTestRunner) Run(root, pkg, tags string, names []string) ([]byte, error) {
	args := []string{"test", "-json", "-count=1", "-tags", tags, "-run", runPattern(names), pkg}
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

// parseOutcomes maps each top-level test name to its final pass/fail/skip
// action. Lines that are not JSON events (toolchain noise) are ignored.
func parseOutcomes(stream []byte) map[string]string {
	outcomes := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var ev testEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Test == "" {
			continue
		}
		switch ev.Action {
		case actionPass, actionFail, actionSkip:
			outcomes[ev.Test] = ev.Action
		}
	}
	return outcomes
}
