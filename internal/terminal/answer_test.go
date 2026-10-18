package terminal_test

import (
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

func TestReadReplyReadsOneAnswerLine(t *testing.T) {
	cases := map[string]terminal.Reply{
		"y\n":       terminal.ReplyYes,
		"  YES  \n": terminal.ReplyYes,
		"no\n":      terminal.ReplyNo,
		"\n":        terminal.ReplyNo,
		"":          terminal.ReplyClosed,
	}
	for input, want := range cases {
		if got := terminal.ReadReply(strings.NewReader(input), nil); got != want {
			t.Errorf("ReadReply(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestReadReplyAbortWinsOverAWaitingRead(t *testing.T) {
	blocked, _ := io.Pipe()
	abort := make(chan struct{})
	close(abort)
	if got := terminal.ReadReply(blocked, abort); got != terminal.ReplyInterrupted {
		t.Fatalf("ReadReply after abort = %d, want ReplyInterrupted", got)
	}
}

func TestTurnLetsOneAskerAtATime(t *testing.T) {
	turn := terminal.NewTurn()
	done, ok := turn.Take(nil)
	if !ok {
		t.Fatal("a free turn was not taken")
	}
	abort := make(chan struct{})
	close(abort)
	if _, ok := turn.Take(abort); ok {
		t.Fatal("a second asker took a turn that was held")
	}
	done()
	if _, ok := turn.Take(nil); !ok {
		t.Fatal("the turn was not handed on")
	}
}
