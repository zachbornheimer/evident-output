package evo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Confirm gate summaries (§ evo-rec.md "confirm gate" default). One spelling
// per outcome so no call site hand-composes the wording a script might parse.
const (
	confirmDeclinedSummary      = "declined"
	confirmPolicyBlockedSummary = "blocked by policy"
	confirmAssumedYesSummary    = "assumed --yes"
	confirmPolicyHint           = "pass --yes to confirm non-interactively"
)

// ConfirmOption configures a Confirm gate.
type ConfirmOption func(*confirmConfig)

type confirmConfig struct {
	assumeYes   bool
	destructive bool
	policyHint  Action
}

// resolvedPolicyHint returns the caller's PolicyHint action, or the default
// "pass --yes" label when the caller didn't set one.
func (c confirmConfig) resolvedPolicyHint() Action {
	if c.policyHint != (Action{}) {
		return c.policyHint
	}
	return Label(confirmPolicyHint)
}

// AssumeYes skips the interactive prompt when v is true (the caller's --yes
// flag). The gate resolves immediately as Done "assumed --yes", and Confirm
// returns true without touching stdin or the live region.
func AssumeYes(v bool) ConfirmOption {
	return func(c *confirmConfig) { c.assumeYes = v }
}

// Destructive annotates the rendered prompt line as "(destructive)".
func Destructive() ConfirmOption {
	return func(c *confirmConfig) { c.destructive = true }
}

// PolicyHint overrides the Next action rendered by Confirm's non-interactive
// policy block. Without this option the block points at "pass --yes to
// confirm non-interactively" — wrong for a caller whose confirm flag isn't
// --yes (e.g. zq clean-repo's --apply). Pass the caller's own executable and
// args so the hint names the flag that actually unblocks it.
func PolicyHint(command string, args ...string) ConfirmOption {
	return func(c *confirmConfig) { c.policyHint = Command(command, args...) }
}

// Confirm asks a yes/no question on the default instance. See Output.Confirm.
func Confirm(question string, opts ...ConfirmOption) bool {
	return Default().Confirm(question, opts...)
}

// Confirm quiesces the live region, renders a durable "?  <question>  [y/N]"
// gate, and reads one answer line — owning the whole gate so no call site
// hand-rolls a [y/N] prompt that fights the spinner or misreports "no" as
// failure (evo-rec.md "confirm gate" default).
//
// Resolution:
//   - AssumeYes(true): Done "assumed --yes", returns true, no prompt.
//   - No TTY / NonInteractive / plain, without AssumeYes: never blocks on
//     stdin — Blocked "blocked by policy" with a Next hint to pass --yes,
//     returns false.
//   - "y"/"yes" (case-insensitive): Done, returns true.
//   - Anything else, including empty: Blocked "declined", returns false —
//     exit 1 via Conclusion precedence, never Failed, never Cancelled.
//   - SIGINT/SIGTERM while waiting: the existing signal path (runInterruptible)
//     cancels the gate — it renders Cancelled, not declined — and Confirm
//     returns false.
func (o *Output) Confirm(question string, opts ...ConfirmOption) bool {
	if o == nil {
		return false
	}
	cfg := confirmConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	gate := o.Task(question)

	if cfg.assumeYes {
		gate.Done(confirmAssumedYesSummary)
		o.flushGateNow(gate.id)
		return true
	}

	if o.cfg.plain || o.cfg.nonInteractive {
		gate.Block(confirmPolicyBlockedSummary)
		gate.Next(cfg.resolvedPolicyHint())
		o.flushGateNow(gate.id)
		return false
	}

	return o.promptConfirm(gate, question, cfg)
}

// flushGateNow locks and forces the immediate durable presentation of a
// resolved Confirm gate — see flushGateNowLocked.
func (o *Output) flushGateNow(id string) {
	o.mu.Lock()
	o.flushGateNowLocked(id)
	o.mu.Unlock()
}

// promptConfirm quiesces the live region for the whole ask-decide-resolve
// window so no live frame can land between the prompt and the durable
// OK/Blocked row — the gate resolves before Suspend resumes any unrelated
// live activity (e.g. a sibling Task still Running).
func (o *Output) promptConfirm(gate *TaskHandle, question string, cfg confirmConfig) bool {
	var yes bool
	_ = o.Suspend(func() error {
		o.writeConfirmPromptLocked(question, cfg.destructive)
		line, cancelled, eof := o.readConfirmLine(gate.id)
		if cancelled {
			// cancelPendingConfirmLocked already resolved the gate as Cancelled.
			return nil
		}
		if eof {
			// Zero-byte EOF on stdin (no interactive human on the other end,
			// e.g. stdin closed or redirected from /dev/null) is a policy
			// block, distinct from a human explicitly typing anything else —
			// evo-rec.md "Confirm EOF = policy block, not decline".
			gate.Block(confirmPolicyBlockedSummary)
			gate.Next(cfg.resolvedPolicyHint())
			o.flushGateNow(gate.id)
			return nil
		}
		yes = isAffirmative(line)
		if yes {
			gate.Done()
		} else {
			gate.Block(confirmDeclinedSummary)
		}
		o.flushGateNow(gate.id)
		return nil
	})
	return yes
}

// writeConfirmPromptLocked emits the durable "?  <question>  [y/N]" line
// above the (now-quiesced) live region.
func (o *Output) writeConfirmPromptLocked(question string, destructive bool) {
	o.mu.Lock()
	color := !o.cfg.noColor
	text := question
	if destructive {
		text += "  (destructive)"
	}
	glyph := styleGlyph(glyphHumanInput.render(o.cfg.glyphs), sgrCyan, color)
	o.writeDurableTextLocked(fmt.Sprintf("%s  %s  [y/N]\n", glyph, text))
	o.mu.Unlock()
}

// readConfirmLine reads one answer line from the Stdin facade, abortable by
// cancelPendingConfirmLocked so a signal unblocks the wait instead of hanging
// until the process is killed a second time. eof reports a zero-byte EOF (no
// data read at all before the stream closed) — distinct from an explicit
// non-yes answer (evo-rec.md "Confirm EOF = policy block, not decline").
func (o *Output) readConfirmLine(itemID string) (line string, cancelled, eof bool) {
	abort := make(chan struct{})
	o.mu.Lock()
	if o.confirmAbort == nil {
		o.confirmAbort = make(map[string]chan struct{})
	}
	o.confirmAbort[itemID] = abort
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.confirmAbort, itemID)
		o.mu.Unlock()
	}()

	type readResult struct {
		text string
		err  error
	}
	result := make(chan readResult, 1)
	go func() {
		reader := bufio.NewReader(o.confirmReader())
		text, err := reader.ReadString('\n')
		result <- readResult{text: text, err: err}
	}()

	select {
	case r := <-result:
		if r.text == "" && errors.Is(r.err, io.EOF) {
			return "", false, true
		}
		return r.text, false, false
	case <-abort:
		return "", true, false
	}
}

// confirmReader returns the Stdin facade, defaulting to os.Stdin — the sole
// place Confirm's logic may name a concrete stream (facade rule).
func (o *Output) confirmReader() io.Reader {
	if o.cfg.stdin != nil {
		return o.cfg.stdin
	}
	return os.Stdin
}

// isAffirmative reports whether an answer line is "y" or "yes" (any case,
// surrounding whitespace ignored). Anything else, including empty, declines.
func isAffirmative(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
