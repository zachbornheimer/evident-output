package evo_test

import (
	"os"
	"strings"
	"testing"
)

// A Get or Wait through a goroutine that exited before anything asked about it
// cannot be traced to the callback that started it: no runtime call names the
// creator of an exited goroutine. CONTRACT.md must say so beside the rule it
// limits, so "in every mode" is never read as covering that shape.
func TestContractStatesWhereGoroutineAttributionEnds(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	const limit = "Attribution ends at a goroutine that exited before anything asked about it"
	if !strings.Contains(text, limit) {
		t.Errorf("CONTRACT.md does not state the limit of goroutine attribution: %q", limit)
	}
}
