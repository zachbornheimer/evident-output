package testkit

import (
	"sync"
	"testing"
)

// TestScreen_ConcurrentAccess proves Screen is safe under concurrent
// live-region writes and reads (X1). Run with `go test -race` — before the
// fix, this trips the race detector on the shared ops/latestLive/finalText
// state.
func TestScreen_ConcurrentAccess(t *testing.T) {
	s := NewScreen(Interactive())

	var wg sync.WaitGroup
	const goroutines = 8
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			s.WriteLive("frame")
			s.ClearLive()
			s.WriteDurable("line")
			s.WriteFinal("final")
			s.SetSize(80+n, 24)
			_ = s.LatestLiveText()
			_ = s.FinalText()
			_ = s.Operations()
			_ = s.LiveFrameCount()
			_ = s.Columns()
			_ = s.Rows()
		}(i)
	}
	wg.Wait()
}
