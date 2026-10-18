package terminal

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// Reply is what one question read from a terminal's input.
type Reply uint8

const (
	// ReplyYes is "y" or "yes", in any case, surrounding space ignored.
	ReplyYes Reply = iota
	// ReplyNo is any other line, an empty one included.
	ReplyNo
	// ReplyClosed is the input closing before a single byte arrived.
	ReplyClosed
	// ReplyInterrupted is abort closing while the question was open.
	ReplyInterrupted
)

// ReadReply reads one answer line from r. abort ends the wait at once, and
// wins over an answer that arrived in the same instant, so an interrupt
// that raced the answer still cancels deterministically. A zero-byte EOF is
// ReplyClosed, distinct from an explicit refusal.
func ReadReply(r io.Reader, abort <-chan struct{}) Reply {
	type line struct {
		text string
		err  error
	}
	read := make(chan line, 1)
	go func() {
		text, err := bufio.NewReader(r).ReadString('\n')
		read <- line{text: text, err: err}
	}()
	select {
	case <-abort:
		return ReplyInterrupted
	default:
	}
	select {
	case <-abort:
		return ReplyInterrupted
	case l := <-read:
		switch {
		case l.text == "" && errors.Is(l.err, io.EOF):
			return ReplyClosed
		case isAffirmative(l.text):
			return ReplyYes
		default:
			return ReplyNo
		}
	}
}

// isAffirmative reports whether line is "y" or "yes" (any case, surrounding
// whitespace ignored). Anything else, including empty, declines.
func isAffirmative(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// Turn is the right to ask a question at the terminal: one prompt shows and
// one reader waits on input at a time. Its zero value is not ready; use
// NewTurn.
type Turn struct{ token chan struct{} }

// NewTurn is a free turn.
func NewTurn() *Turn { return &Turn{token: make(chan struct{}, 1)} }

// Take waits for the turn. abort ends the wait with ok false. done hands the
// turn to the next asker.
func (t *Turn) Take(abort <-chan struct{}) (done func(), ok bool) {
	select {
	case t.token <- struct{}{}:
		return func() { <-t.token }, true
	case <-abort:
		return nil, false
	}
}
