package evo

import (
	"bytes"
	"testing"
)

// FormatData's whole contract is "domain payload on stdout, presentation on
// stderr", and the only way a caller can hold up its end is to be handed the
// payload stream. The rec-dialect rename made that method unexported while
// every doc comment kept pointing at the exported name, so callers could
// configure FormatData and then had nowhere to write — zq went back to a raw
// os.Stdout write and spliced its own task rows into its JSON.
func TestResultWriter_FormatDataHandsTheCallerItsPayloadStream(t *testing.T) {
	var payload, human bytes.Buffer
	out := initRun(Config{Name: "zq", Facades: Facades{Stdout: &payload, Stderr: &human}})

	succeed(out.Task("gitleaks"))
	if _, err := out.ResultWriter().Write([]byte(`{"findings":[]}`)); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := payload.String(); got != `{"findings":[]}` {
		t.Fatalf("payload stream = %q; presentation must never reach it", got)
	}
	if human.Len() == 0 {
		t.Fatal("presentation vanished; FormatData routes it to stderr, it does not drop it")
	}
}
