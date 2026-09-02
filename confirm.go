package evo

import (
	"bufio"
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
	confirmAssumedYesBecause    = "assumed --yes"
	confirmPolicyHint           = "pass --yes to confirm non-interactively"
)

// ConfirmOption configures a Confirm gate.
type ConfirmOption func(*confirmConfig)

type confirmConfig struct {
	assumeYes   bool
	destructive bool
}

// AssumeYes skips the interactive prompt when v is true (the caller's --yes
// flag). The gate resolves immediately as OK, annotated "assumed --yes",
// and Confirm returns true without touching stdin or the live region.
func AssumeYes(v bool) ConfirmOption {
	return func(c *confirmConfig) { c.assumeYes = v }
}

// Destructive annotates the rendered prompt line as "(destructive)".
func Destructive() ConfirmOption {
	return func(c *confirmConfig) { c.destructive = true }
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
//   - AssumeYes(true): OK "assumed --yes", returns true, no prompt.
//   - No TTY / NonInteractive / plain, without AssumeYes: never blocks on
//     stdin — Blocked "blocked by policy" with a Next hint to pass --yes,
//     returns false.
//   - "y"/"yes" (case-insensitive): OK, returns true.
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

	item := o.Item(question)

	if cfg.assumeYes {
		item.OK()
		item.Because(confirmAssumedYesBecause)
		return true
	}

	if o.cfg.plain || o.cfg.nonInteractive {
		item.Block(confirmPolicyBlockedSummary).Next(Label(confirmPolicyHint))
		return false
	}

	return o.promptConfirm(item, question, cfg.destructive)
}

// promptConfirm quiesces the live region for the whole ask-decide-resolve
// window so no live frame can land between the prompt and the durable
// OK/Blocked row — the gate resolves before Suspend resumes any unrelated
// live activity (e.g. a sibling Task still Running).
func (o *Output) promptConfirm(item *ItemHandle, question string, destructive bool) bool {
	var yes bool
	_ = o.Suspend(func() error {
		o.writeConfirmPromptLocked(question, destructive)
		line, cancelled := o.readConfirmLine(item.id)
		if cancelled {
			// cancelPendingConfirmLocked already resolved the gate as Cancelled.
			return nil
		}
		yes = isAffirmative(line)
		if yes {
			item.OK()
		} else {
			item.Block(confirmDeclinedSummary)
		}
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
	glyph := styleGlyph("?", sgrCyan, color)
	o.writeDurableTextLocked(fmt.Sprintf("%s  %s  [y/N]\n", glyph, text))
	o.mu.Unlock()
}

// readConfirmLine reads one answer line from the Stdin facade, abortable by
// cancelPendingConfirmLocked so a signal unblocks the wait instead of hanging
// until the process is killed a second time.
func (o *Output) readConfirmLine(itemID string) (line string, cancelled bool) {
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

	result := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(o.confirmReader())
		text, _ := reader.ReadString('\n')
		result <- text
	}()

	select {
	case text := <-result:
		return text, false
	case <-abort:
		return "", true
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
