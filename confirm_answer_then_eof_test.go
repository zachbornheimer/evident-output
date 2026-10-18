package evo

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestConfirmAcceptsAnswerThenEOF pins that an answer followed by end of input is the answer, however the
// input arrives: stdin closes right after the user's line when it is piped or a terminal session ends.
// Only an end of input with no answer at all is "no answer — stdin closed".
//
// A review probe once saw a refusal under script(1). That was the harness: with piped input, script(1)
// writes an end-of-file character to the terminal ahead of the piped answer, so the program really did see
// the input close before any answer. Held open until the program exits, the same answer is accepted.
func TestConfirmAcceptsAnswerThenEOF(t *testing.T) {
	cases := []struct {
		name  string
		stdin io.Reader
		want  bool
		// closed is whether the refusal says stdin closed with no answer.
		closed bool
	}{
		{"answer line then EOF", strings.NewReader("y\n"), true, false},
		{"answer without a newline then EOF", strings.NewReader("y"), true, false},
		{"word answer with a carriage return", strings.NewReader("yes\r\n"), true, false},
		{"answer and EOF delivered by one read", iotest.DataErrReader(strings.NewReader("y\n")), true, false},
		{"answer delivered one byte at a time", iotest.OneByteReader(strings.NewReader("y\n")), true, false},
		{"refusal then EOF", strings.NewReader("n\n"), false, false},
		{"EOF with no answer", strings.NewReader(""), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
			out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: tc.stdin}}, tuning{color: colorNever})
			t.Cleanup(func() { _ = out.close() })

			if got := out.Confirm("proceed?"); got != tc.want {
				t.Fatalf("Confirm = %v, want %v", got, tc.want)
			}

			problems := out.snapshot().Tasks[0].Problems
			closed := len(problems) > 0 && problems[0].Summary == "no answer — stdin closed"
			if closed != tc.closed {
				t.Errorf("stdin-closed refusal = %v, want %v (problems %+v)", closed, tc.closed, problems)
			}
		})
	}
}
