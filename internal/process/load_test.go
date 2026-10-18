package process

import (
	"runtime"
	"testing"
)

func TestLoadAverageIsANonNegativeReading(t *testing.T) {
	load, ok := LoadAverage()
	if !ok {
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			t.Fatalf("LoadAverage reported no reading on %s", runtime.GOOS)
		}
		return
	}
	if load < 0 {
		t.Fatalf("LoadAverage = %v, want >= 0", load)
	}
}
