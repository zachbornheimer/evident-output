package terminal

import (
	"os"
	"strings"
	"testing"
)

func TestReaderIsTerminal(t *testing.T) {
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeRead.Close(); _ = pipeWrite.Close() })
	device, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = device.Close() })

	cases := map[string]struct {
		reader interface{ Read([]byte) (int, error) }
		want   bool
	}{
		"null device":      {device, false},
		"pipe":             {pipeRead, false},
		"in-memory reader": {strings.NewReader("y\n"), false},
		"nil":              {nil, false},
	}
	for name, tc := range cases {
		if got := ReaderIsTerminal(tc.reader); got != tc.want {
			t.Errorf("ReaderIsTerminal(%s) = %v, want %v", name, got, tc.want)
		}
	}
}
