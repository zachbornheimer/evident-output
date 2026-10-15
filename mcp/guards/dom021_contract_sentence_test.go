package guards_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// noDefineSentenceRE finds CONTRACT.md's rule for a Task nobody Defined.
var noDefineSentenceRE = regexp.MustCompile(`A Task with no Define[^.]*\.`)

// TestDOM021TextIsTheContractSentence pins that the MCP says what the runtime
// does: DOM-021's explanation and each finding it raises carry CONTRACT.md's
// sentence for a Task with no Define, word for word, so the contract and the
// rule cannot drift apart.
func TestDOM021TextIsTheContractSentence(t *testing.T) {
	contract, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	sentence := noDefineSentenceRE.FindString(strings.Join(strings.Fields(string(contract)), " "))
	if sentence == "" {
		t.Fatal("CONTRACT.md has no sentence starting \"A Task with no Define\"")
	}

	rule, ok := rules.Explain("DOM-021")
	if !ok {
		t.Fatal("DOM-021 is not registered")
	}
	if !strings.Contains(rule.Why, sentence) {
		t.Errorf("DOM-021 Why does not carry the CONTRACT.md sentence\nwant: %s\ngot:  %s", sentence, rule.Why)
	}

	res := review.GoSource("main.go", "package main\n\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\n\nfunc run(r *evo.Run) {\n\tr.Task(\"bare\")\n}\n")
	for _, f := range res.Findings {
		if f.RuleID == "DOM-021" {
			if !strings.Contains(f.Message, sentence) {
				t.Errorf("DOM-021 finding does not carry the CONTRACT.md sentence\nwant: %s\ngot:  %s", sentence, f.Message)
			}
			return
		}
	}
	t.Error("DOM-021 did not fire on a bare r.Task(\"bare\") statement")
}
