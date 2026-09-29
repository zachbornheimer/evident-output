package retired

import "testing"

func TestWarnTaughtSparesSlog(t *testing.T) {
	for text, want := range map[string]bool{
		"task.Warn(\"stale\")":            true,
		"evo.Warn(\"disk nearly full\")":  true,
		"a Fail/Warn/Block summary":       true,
		"logger.Warn(\"slow\", \"d\", 4)": false,
		"slog.LevelWarn":                  false,
		"Info/Warn/Error keep time":       false,
	} {
		if got := warnTaught.MatchString(text); got != want {
			t.Errorf("warnTaught(%q) = %v, want %v", text, got, want)
		}
	}
}
