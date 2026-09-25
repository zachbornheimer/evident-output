package retired

import "testing"

func TestUnexplainedInAllowsARemovalNote(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"use task.Record(verb, n, object) when the work already happened", 1},
		{"Record was removed in 1.1; task.Record(...) becomes evo.Effect", 0},
		{"range Group.Each so every iteration is visible", 1},
		{"Group.Each was removed in 1.0", 0},
		{"Group.Each was removed in 1.1", 1},
		{"Doing/Progress/Done already activate the task", 1},
		{"select { case <-ctx.Done(): }", 0},
	}
	for _, c := range cases {
		if got := len(UnexplainedIn(c.text)); got != c.want {
			t.Errorf("UnexplainedIn(%q) = %d hits, want %d", c.text, got, c.want)
		}
	}
}

func TestEverySymbolHasAReleaseAndReplacement(t *testing.T) {
	for _, s := range Symbols() {
		if s.Replacement == "" || s.RemovedIn == "" {
			t.Errorf("%s: Replacement and RemovedIn are required", s.Contract)
		}
	}
}

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
