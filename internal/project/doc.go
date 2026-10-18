// Package project decides how a run is shown: the shared row model, live
// TTY frames, durable plain text, and the evo.run / evo.event wire.
//
// It imports only the standard library, internal/record (read-only), and
// the terminal and clock facades. Scheduling, freshness and change stay
// out of it; they write record, and project reads what they wrote.
package project
