package evo_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// journalEnabled reports, for each level, whether a run configured with cfg journals slog records at it, and
// returns the text the run printed after a record at every level.
func journalEnabled(t *testing.T, cfg evo.Config, levels ...slog.Level) (enabled []bool, printed string) {
	t.Helper()
	cfg.Output = evo.Plain
	run := executeContract(context.Background(), t, cfg, func(ctx context.Context, r *evo.Run) error {
		handler := r.SlogHandler()
		logger := slog.New(handler)
		for _, level := range levels {
			enabled = append(enabled, handler.Enabled(ctx, level))
			logger.Log(ctx, level, "record at "+level.String())
		}
		return nil
	})
	return enabled, run.Text()
}

// TestZeroDebugIsSilentAndExplicitLevelWins pins CONTRACT section 6: the zero Config.Debug keeps the journal
// off, so no slog record prints, and EVO_DEBUG fills in only that zero. A Debug level the program names,
// slog.LevelInfo included, beats EVO_DEBUG, as Config.Output beats EVO_OUTPUT.
func TestZeroDebugIsSilentAndExplicitLevelWins(t *testing.T) {
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}

	t.Run("zero Debug journals nothing", func(t *testing.T) {
		withEnvironment(t, nil)
		enabled, printed := journalEnabled(t, evo.Config{}, levels...)
		for i, level := range levels {
			if enabled[i] {
				t.Errorf("zero Config.Debug journals %s records", level)
			}
		}
		if strings.Contains(printed, "record at") {
			t.Errorf("zero Config.Debug printed slog records:\n%s", printed)
		}
	})

	t.Run("EVO_DEBUG fills in the zero Debug", func(t *testing.T) {
		withEnvironment(t, map[string]string{"EVO_DEBUG": "warn"})
		enabled, _ := journalEnabled(t, evo.Config{}, levels...)
		if want := []bool{false, false, true, true}; !equalBools(enabled, want) {
			t.Errorf("EVO_DEBUG=warn with zero Debug journals %v at debug, info, warn, error; want %v", enabled, want)
		}
	})

	t.Run("an explicit level beats EVO_DEBUG", func(t *testing.T) {
		withEnvironment(t, map[string]string{"EVO_DEBUG": "debug"})
		enabled, printed := journalEnabled(t, evo.Config{Debug: slog.LevelInfo}, levels...)
		if want := []bool{false, true, true, true}; !equalBools(enabled, want) {
			t.Errorf("Debug: slog.LevelInfo under EVO_DEBUG=debug journals %v at debug, info, warn, error; want %v", enabled, want)
		}
		if !strings.Contains(printed, "record at INFO") {
			t.Errorf("an explicit Info level printed no Info record:\n%s", printed)
		}
	})
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTypedNilDebugMeansOff pins CONTRACT section 6 for a Debug interface holding a nil pointer, such as an
// unset *slog.LevelVar field: it is the zero Debug, so the journal stays off and EVO_DEBUG fills it in, rather
// than Execute dereferencing the nil pointer.
func TestTypedNilDebugMeansOff(t *testing.T) {
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	var unset *slog.LevelVar

	t.Run("journals nothing", func(t *testing.T) {
		withEnvironment(t, nil)
		enabled, printed := journalEnabled(t, evo.Config{Debug: unset}, levels...)
		for i, level := range levels {
			if enabled[i] {
				t.Errorf("a typed-nil Config.Debug journals %s records", level)
			}
		}
		if strings.Contains(printed, "record at") {
			t.Errorf("a typed-nil Config.Debug printed slog records:\n%s", printed)
		}
	})

	t.Run("EVO_DEBUG fills it in like the zero Debug", func(t *testing.T) {
		withEnvironment(t, map[string]string{"EVO_DEBUG": "warn"})
		enabled, _ := journalEnabled(t, evo.Config{Debug: unset}, levels...)
		if want := []bool{false, false, true, true}; !equalBools(enabled, want) {
			t.Errorf("EVO_DEBUG=warn with a typed-nil Debug journals %v at debug, info, warn, error; want %v", enabled, want)
		}
	})
}
