package main

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// failingResponse is a client that went away: every body write fails.
type failingResponse struct{ header http.Header }

func (f *failingResponse) Header() http.Header       { return f.header }
func (f *failingResponse) WriteHeader(int)           {}
func (f *failingResponse) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// An undelivered response is logged with an id that tells one request
// from another. run_id cannot: it is out_1 on every 1.2 run.
func TestLaunchHTTP_UndeliveredResponseLogsTheRequestID(t *testing.T) {
	var logs bytes.Buffer
	h := runHandler{
		agent: newAgent(t.TempDir()), stateDir: t.TempDir(), budget: testBudget,
		admission: newAdmission(1), log: slog.New(slog.NewTextHandler(&logs, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(requestIDHeader, "req-7f3a")

	h.ServeHTTP(&failingResponse{header: http.Header{}}, req)

	line := logs.String()
	if !strings.Contains(line, "request_id=req-7f3a") || strings.Contains(line, "run_id") {
		t.Fatalf("log = %q, want request_id=req-7f3a and no run_id", line)
	}
}

// A request without the header still gets an id, distinct per request.
func TestRequestIDFor_GeneratesOneWhenTheClientSentNone(t *testing.T) {
	first := requestIDFor(httptest.NewRequest(http.MethodPost, "/", nil))
	second := requestIDFor(httptest.NewRequest(http.MethodPost, "/", nil))
	if first == "" || first == second {
		t.Fatalf("generated ids %q and %q, want two distinct non-empty ids", first, second)
	}
}
