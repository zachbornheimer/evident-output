package tests22_test

import (
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

const (
	brailleFirst, brailleLast = 0x2800, 0x28FF
	screenWidth               = 80
)

var elapsedSeconds = regexp.MustCompile(`\d+s`)

// withoutSpinner drops the animated braille glyph and masks the elapsed
// seconds so two frames compare on layout alone.
func withoutSpinner(frame string) string {
	stripped := strings.Map(func(r rune) rune {
		if r >= brailleFirst && r <= brailleLast {
			return -1
		}
		return r
	}, frame)
	return elapsedSeconds.ReplaceAllString(stripped, "Ns")
}

// liveFrameAt advances the domain clock by step, repaints, and returns the
// live frame with its spinner and seconds masked.
func liveFrameAt(screen *testkit.Screen, clock *testkit.Clock, repaint func(), step time.Duration) string {
	clock.Advance(step)
	repaint()
	return withoutSpinner(screen.LatestLiveText())
}

func TestC22_008_TimerAndCurrentItemLayoutStayPutAsSecondsTick(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(screenWidth), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.Delay(0), Color: evo.ColorNever,
	})
	t.Cleanup(func() { _ = out.Close() })

	push, ticker := out.Task("push"), out.Task("ticker")
	push.Doing("pushing feat/style-contract")
	push.Progress(3, 10)
	repaint := func() { ticker.Progress(1, 100) }
	repaint()

	const beforeRollover, afterRollover = 9 * time.Second, time.Second
	nineSeconds := liveFrameAt(screen, clock, repaint, beforeRollover)
	tenSeconds := liveFrameAt(screen, clock, repaint, afterRollover)
	if !strings.Contains(nineSeconds, "Ns") {
		t.Fatalf("no timer on the row after %s:\n%s", beforeRollover, nineSeconds)
	}
	if nineSeconds != tenSeconds {
		t.Fatalf("layout moved when the timer went from one digit to two:\n--- 9s ---\n%s\n--- 10s ---\n%s", nineSeconds, tenSeconds)
	}
}
