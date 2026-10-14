package terminal

import (
	"os"
	"strings"
	"testing"
)

func TestReaderIsCharDevice(t *testing.T) {
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
		"character device": {device, true},
		"pipe":             {pipeRead, false},
		"in-memory reader": {strings.NewReader("y\n"), false},
		"nil":              {nil, false},
	}
	for name, tc := range cases {
		if got := ReaderIsCharDevice(tc.reader); got != tc.want {
			t.Errorf("ReaderIsCharDevice(%s) = %v, want %v", name, got, tc.want)
		}
	}
}
