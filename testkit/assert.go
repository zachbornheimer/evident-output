package testkit

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// RequireConclusion asserts the output conclusion state.
func RequireConclusion(t *testing.T, out *evo.Output, want evo.ConclusionState) {
	t.Helper()
	got := out.Conclusion()
	if got.State != want {
		t.Fatalf("conclusion state = %q, want %q", got.State, want)
	}
}

// RequireClean asserts no misuse error is recorded.
func RequireClean(t *testing.T, out *evo.Output) {
	t.Helper()
	if err := out.Err(); err != nil {
		t.Fatalf("output misuse error: %v", err)
	}
}

// UnreadableStdin returns a reader that fails the test the moment anything
// reads from it — used to prove a code path never touches stdin.
func UnreadableStdin(t *testing.T) io.Reader {
	return &unreadableStdin{t: t}
}

type unreadableStdin struct{ t *testing.T }

func (r *unreadableStdin) Read(_ []byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("read from stdin when it should not have")
	return 0, io.EOF
}
