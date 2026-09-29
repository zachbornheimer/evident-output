package main

import (
	"testing"
	"time"
)

func TestResolvePresentation_AutoWithTerminal_ReturnsTTY(t *testing.T) {
	t.Parallel()
	got, err := resolvePresentation(modeAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != modeTTY {
		t.Fatalf("mode = %q, want %q", got, modeTTY)
	}
}

func TestResolvePresentation_AutoWithoutTerminal_ReturnsStream(t *testing.T) {
	t.Parallel()
	got, err := resolvePresentation(modeAuto, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != modeStream {
		t.Fatalf("mode = %q, want %q", got, modeStream)
	}
}

func TestResolvePresentation_ExplicitTTY_StaysTTYWhenStdoutIsNotTerminal(t *testing.T) {
	t.Parallel()
	got, err := resolvePresentation(modeTTY, false)
	if err != nil {
		t.Fatal(err)
	}
	if got != modeTTY {
		t.Fatalf("mode = %q, want %q", got, modeTTY)
	}
}

func TestResolvePresentation_ExplicitStream_StaysStreamWhenStdoutIsTerminal(t *testing.T) {
	t.Parallel()
	got, err := resolvePresentation(modeStream, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != modeStream {
		t.Fatalf("mode = %q, want %q", got, modeStream)
	}
}

func TestResolvePresentation_InvalidMode_Errors(t *testing.T) {
	t.Parallel()
	if _, err := resolvePresentation("json", true); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestPresentation_TTY_DisablesEvoPlainAndRawPassthrough(t *testing.T) {
	t.Parallel()
	mode, err := resolvePresentation(modeTTY, false)
	if err != nil {
		t.Fatal(err)
	}
	if streamMode(mode) {
		t.Fatal("tty must set evo Plain false and raw child passthrough false")
	}
}

func TestPresentation_Stream_EnablesEvoPlainAndRawPassthrough(t *testing.T) {
	t.Parallel()
	mode, err := resolvePresentation(modeStream, true)
	if err != nil {
		t.Fatal(err)
	}
	if !streamMode(mode) {
		t.Fatal("stream must set evo Plain true and raw child passthrough true")
	}
}

func TestNextHeartbeatWait_ActivityJustAfterTick_DoesNotDeferToDoubleInterval(t *testing.T) {
	t.Parallel()
	interval := 2 * time.Second
	tick := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	last := tick.Add(10 * time.Millisecond)
	now := last
	wait := nextHeartbeatWait(now, last, interval)
	if wait != interval {
		t.Fatalf("wait = %s, want %s from last activity (not ~2x ticker remainder)", wait, interval)
	}

	now = tick.Add(interval + 10*time.Millisecond)
	wait = nextHeartbeatWait(now, last, interval)
	if wait != 0 {
		t.Fatalf("wait = %s, want 0 once last activity is a full interval old", wait)
	}
}

func TestNormalizeConfig_EmptyMode_BecomesAuto(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = ""
	if err := normalizeConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != modeAuto {
		t.Fatalf("mode = %q, want %q", cfg.Mode, modeAuto)
	}
}

func TestNormalizeConfig_InvalidMode_Errors(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = "pretty"
	if err := normalizeConfig(&cfg); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestSmokeConfig_UsesAutoMode(t *testing.T) {
	t.Parallel()
	cfg := smokeConfig()
	if cfg.Mode != modeAuto {
		t.Fatalf("smoke mode = %q, want %q", cfg.Mode, modeAuto)
	}
}
