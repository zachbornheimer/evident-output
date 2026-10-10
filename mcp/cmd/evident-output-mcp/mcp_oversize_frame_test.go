package main

import (
	"fmt"
	"strings"
	"testing"
)

// oversizeBytes is one byte past the frame limit.
const oversizeBytes = maxFrameBytes + 1

// An oversize request gets a JSON-RPC error and the server keeps serving:
// the next request still gets its reply.
func TestOversizeNDJSONFrameIsAnsweredNotFatal(t *testing.T) {
	bin := buildMCP(t)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping","params":{"pad":"` + strings.Repeat("x", oversizeBytes) + `"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"ping"}` + "\n"
	assertOversizeAnswered(t, runMCP(t, bin, in))
}

func TestOversizeContentLengthFrameIsAnsweredNotFatal(t *testing.T) {
	bin := buildMCP(t)
	body := strings.Repeat("x", oversizeBytes)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body) +
		`{"jsonrpc":"2.0","id":3,"method":"ping"}` + "\n"
	assertOversizeAnswered(t, runMCP(t, bin, in))
}

func assertOversizeAnswered(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, `"code":-32600`) {
		t.Fatalf("oversize frame got no -32600 error: %s", out)
	}
	if !strings.Contains(out, `"id":3`) {
		t.Fatalf("ping after an oversize frame got no reply; the server died: %s", out)
	}
}
