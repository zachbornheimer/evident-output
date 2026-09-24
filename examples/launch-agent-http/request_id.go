package main

import (
	"crypto/rand"
	"net/http"
)

// requestIDHeader carries the id a client or proxy assigned the request.
const requestIDHeader = "X-Request-Id"

// requestIDFor is the id the host correlates one request by: the
// client's own X-Request-Id, else a fresh random one. evo's run_id cannot
// serve: it is out_1 on every 1.2 run (docs/migration/1.2.md).
func requestIDFor(r *http.Request) string {
	if id := r.Header.Get(requestIDHeader); id != "" {
		return id
	}
	return rand.Text()
}
