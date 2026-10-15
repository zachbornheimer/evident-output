package testkit

import (
	"io"
	"testing"
)

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
