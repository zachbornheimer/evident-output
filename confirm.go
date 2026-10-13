package evo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/graph"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Confirm gate summaries (§ evo-rec.md "confirm gate" default). One spelling
// per outcome so no call site hand-composes the wording a script might parse.
const (
	confirmDeclinedSummary      = "declined"
	confirmPolicyBlockedSummary = "blocked by policy"
	confirmAssumedYesSummary    = "assumed --yes"
	// confirmEOFSummary is the rendered detail for a zero-byte EOF on stdin
	// (release-gate round 4 finding 6) — a distinct sentence from
	// confirmPolicyBlockedSummary's no-TTY/plain/NonInteractive policy block,
	// so the printed reason names what actually happened (stdin closed
	// before any answer arrived) instead of reusing wording that implies a
	// deliberate policy decision.
	confirmEOFSummary = "no answer — stdin closed"
	confirmPolicyHint = "pass --yes to confirm non-interactively"
	// confirmInputLineGlyph is the P11 input line rendered under the question
	// — the typed answer lands here, on its own line, so the question and
	// choices never compete visually with the human's keystrokes. No
	// trailing newline: the human's answer (and their terminal's own
	// newline on Enter) continues this same line.
	confirmInputLineGlyph = "›"
)

// ConfirmOption configures a Confirm gate.
type ConfirmOption func(*confirmConfig)

type confirmConfig struct {
	assumeYes   bool
	destructive bool
	policyHint  Action
	// policyFlag is PolicyFlag's flag spelling, resolved into a Command
	// against the Output's own identity at read time (resolvedPolicyHint) —
	// not at option-build time, since only the Output knows its Title/
	// executable identity.
	policyFlag string
	// detail holds ConfirmDetail's context lines, rendered under the
	// "?  <question>  [y/N]" prompt line.
	detail []string
}

// resolvedPolicyHint returns the caller's PolicyHint action, PolicyFlag
// resolved against o's own identity, or the default "pass --yes" label when
// the caller set neither.
func (c confirmConfig) resolvedPolicyHint(o *Run) Action {
	if c.policyHint != (Action{}) {
		return c.policyHint
	}
	if c.policyFlag != "" {
		return actionCommand(o.policySourceName(), c.policyFlag)
	}
	return label(confirmPolicyHint)
}

// policySourceName names the executable a PolicyFlag hint points at: the
// caller's own Config.Title when set, else the binary's own basename (I2's
// identityFallbackName) — the same identity a caller would otherwise have
// to hand-compose via PolicyHint(os.Args[0], flag).
func (o *Run) policySourceName() string {
	if o.cfg.subject != "" {
		return o.cfg.subject
	}
	return identityFallbackName()
}

// AssumeYes skips the interactive prompt when v is true (the caller's --yes
// flag). The gate resolves immediately as Done "assumed --yes", and Confirm
// returns true without touching stdin or the live region.
func assumeYes(v bool) ConfirmOption {
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
func policyHint(command string, args ...string) ConfirmOption {
	return func(c *confirmConfig) { c.policyHint = actionCommand(command, args...) }
}

// PolicyFlag sets the non-interactive policy hint to a flag on the caller's
// own executable (e.g. "--apply") instead of PolicyHint's explicit foreign-
// command spelling — the common case where the flag that unblocks a
// non-interactive run belongs to the very binary calling Confirm. The
// executable name is resolved from the same identity Config.Title/I2's
// executable-basename fallback uses. Reach for PolicyHint instead when the
// hint should point at a different tool.
func policyFlag(flag string) ConfirmOption {
	return func(c *confirmConfig) { c.policyFlag = flag }
}

// ConfirmDetail attaches one or more context lines rendered under the
// "?  <question>  [y/N]" prompt line — e.g. what will actually happen,
// which host is affected. Repeated calls append.
func confirmDetail(lines ...string) ConfirmOption {
	return func(c *confirmConfig) { c.detail = append(c.detail, lines...) }
}

// Confirm asks a yes/no question on the default instance. See Output.Confirm.
func confirm(question string, opts ...ConfirmOption) bool {
	return defaultRun().Confirm(question, opts...)
}

// Confirm quiesces the live region, renders a durable "?  <question>  [y/N]"
// gate, and reads one answer line — owning the whole gate so no call site
// hand-rolls a [y/N] prompt that fights the spinner or misreports "no" as
// failure (evo-rec.md "confirm gate" default).
//
// question is verbatim text, not a printf format (unlike Task/Group/Reason's
// C6 name argument) — build a dynamic question with fmt.Sprintf before
// calling Confirm.
//
// Resolution:
//   - AssumeYes(true): Done "assumed --yes", returns true, no prompt.
//   - No TTY / NonInteractive / plain, without AssumeYes: never blocks on
//     stdin — Blocked "blocked by policy" with a Next hint to pass --yes,
//     returns false.
//   - "y"/"yes" (case-insensitive): Done, returns true.
//   - Anything else, including empty: Blocked "declined", returns false —
//     exit 1 via Conclusion precedence, never Failed, never Cancelled.
//   - Zero-byte EOF on stdin (stdin closed or redirected from /dev/null
//     before any answer arrived): Blocked "no answer — stdin closed" with
//     the same --yes Next hint, returns false — distinct wording from the
//     no-TTY policy block above, since nothing decided to refuse; the stream
//     simply gave no answer.
//   - SIGINT/SIGTERM while waiting: the existing signal path (runInterruptible)
//     cancels the gate — it renders Cancelled, not declined — and Confirm
//     returns false.
func (o *Run) Confirm(question string, opts ...ConfirmOption) bool {
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

	switch {
	case cfg.assumeYes:
		return o.settleGate(gate, graph.AnswerAssumedYes, cfg)
	case o.cfg.plain:
		return o.settleGate(gate, graph.AnswerPolicy, cfg)
	}

	return o.promptConfirm(gate, question, cfg)
}

// settleGate resolves the Confirm gate as its answer decides (see
// graph.Answer.Outcome) and reports whether the answer was yes. The words
// that say why a gate was refused are the presentation's; which state it
// reaches is the graph's.
func (o *Run) settleGate(gate *Task, answer graph.Answer, cfg confirmConfig) bool {
	switch answer.Outcome() {
	case cancelled:
		// cancelPendingConfirmLocked already resolved the gate as Cancelled.
		return false
	case done:
		gate.succeed(confirmSummaryOf(answer))
		o.flushGateNow(gate.id)
		return true
	default:
		gate.block(confirmSummaryOf(answer), o.confirmBlockOptions(answer, cfg)...)
		o.flushGateNow(gate.id)
		return false
	}
}

// confirmSummaryOf is the one spelling of each answer's row detail.
func confirmSummaryOf(answer graph.Answer) string {
	switch answer {
	case graph.AnswerAssumedYes:
		return confirmAssumedYesSummary
	case graph.AnswerNo:
		return confirmDeclinedSummary
	case graph.AnswerPolicy:
		return confirmPolicyBlockedSummary
	case graph.AnswerClosed:
		return confirmEOFSummary
	default:
		return ""
	}
}

// confirmBlockOptions is the Next hint an answer that nobody gave on purpose
// (a policy block, a closed stdin) points at: how to confirm non-interactively.
func (o *Run) confirmBlockOptions(answer graph.Answer, cfg confirmConfig) []ProblemOption {
	if answer == graph.AnswerPolicy || answer == graph.AnswerClosed {
		return []ProblemOption{Next(cfg.resolvedPolicyHint(o))}
	}
	return nil
}

// flushGateNow locks and forces the immediate durable presentation of a
// resolved Confirm gate — see commitResolvedTaskLocked.
func (o *Run) flushGateNow(id string) {
	o.mu.Lock()
	o.commitResolvedTaskLocked(id)
	o.mu.Unlock()
}

// promptConfirm quiesces the live region for the whole ask-decide-resolve
// window so no live frame can land between the prompt and the durable
// OK/Blocked row — the gate resolves before Suspend repaints the live region
// for the siblings still Running. Siblings are not held: they keep running,
// and their rows paint once the window closes.
//
// The abort channel is registered here, before the prompt is written or
// Suspend touches the live region — not lazily inside readConfirmLine.
// Registering it late left a window, from gate creation through the
// durable prompt write, where a SIGINT had no abort channel to close: it
// fell through cancelActive's generic Running/Pending scan, which marked
// the gate Cancelled without ever unblocking the stdin read that was about
// to start — a swallowed ^C that hung waiting for an answer nothing could
// interrupt (X2).
func (o *Run) promptConfirm(gate *Task, question string, cfg confirmConfig) bool {
	abort, release := o.graph.RegisterAbort(gate.id)
	defer release()

	endTurn, ok := o.awaitConfirmTurn(abort)
	if !ok {
		return o.settleGate(gate, graph.AnswerInterrupted, cfg)
	}
	defer endTurn()

	o.setOwnsWindow(gate.id, true)
	defer o.setOwnsWindow(gate.id, false)

	var yes bool
	_ = o.Suspend(func() error {
		o.writeConfirmPrompt(question, cfg.destructive, cfg.detail)
		yes = o.settleGate(gate, o.readConfirmAnswer(abort), cfg)
		return nil
	})
	return yes
}

// awaitConfirmTurn waits until no other Confirm owns the terminal, so only one
// prompt window is open and only one reader waits on stdin. abort ends the
// wait: ok is false and the gate was already cancelled. endTurn hands the
// terminal to the next Confirm.
func (o *Run) awaitConfirmTurn(abort <-chan struct{}) (endTurn func(), ok bool) {
	o.mu.Lock()
	if o.confirmTurn == nil {
		o.confirmTurn = make(chan struct{}, 1)
	}
	turn := o.confirmTurn
	o.mu.Unlock()
	select {
	case turn <- struct{}{}:
		return func() { <-turn }, true
	case <-abort:
		return nil, false
	}
}

// setOwnsWindow marks the Confirm gate id as the Task its Suspend window
// shows, or clears the mark (see taskState.ownsWindow).
func (o *Run) setOwnsWindow(id string, owns bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskStates[id]; st != nil {
		st.ownsWindow = owns
	}
}

// writeConfirmPrompt emits the durable "?  <question>  [y/N]" line,
// plus any ConfirmDetail context lines beneath it, above the (now-quiesced)
// live region.
func (o *Run) writeConfirmPrompt(question string, destructive bool, detail []string) {
	o.mu.Lock()
	color := !o.cfg.noColor
	text := question
	if destructive {
		text += "  (destructive)"
	}
	glyph := txt.StyleGlyph(txt.GlyphHumanInput.Render(o.cfg.glyphs), txt.SGRCyan, color)
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  [y/N]\n", glyph, text)
	for _, line := range detail {
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "  %s\n", txt.Dim(line, color))
	}
	// P11: the typed answer lands on its own input line, below the question,
	// so the question/choices and the human's keystrokes never compete on one
	// line — "? question [y/N] y" is exactly what this replaces.
	b.WriteString(confirmInputLineGlyph)
	o.writeWindowOwnerTextLocked(b.String())
	o.mu.Unlock()
}

// readConfirmAnswer reads one answer line from the Stdin facade, abortable via
// abort (registered by promptConfirm at gate creation) so a signal unblocks
// the wait instead of hanging until the process is killed a second time. A
// zero-byte EOF (no data read at all before the stream closed) is
// graph.AnswerClosed, distinct from an explicit non-yes answer (evo-rec.md
// "Confirm EOF = policy block, not decline").
func (o *Run) readConfirmAnswer(abort <-chan struct{}) graph.Answer {
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

	// abort takes priority when both are ready: a signal that raced the
	// answer to the finish line still cancels the gate, deterministically —
	// never left to select's random choice between two ready cases.
	select {
	case <-abort:
		return graph.AnswerInterrupted
	default:
	}

	select {
	case <-abort:
		return graph.AnswerInterrupted
	case r := <-result:
		if r.text == "" && errors.Is(r.err, io.EOF) {
			return graph.AnswerClosed
		}
		if isAffirmative(r.text) {
			return graph.AnswerYes
		}
		return graph.AnswerNo
	}
}

// confirmReader returns the Stdin facade, defaulting to os.Stdin — the sole
// place Confirm's logic may name a concrete stream (facade rule).
func (o *Run) confirmReader() io.Reader {
	if o.cfg.stdin != nil {
		return o.cfg.stdin
	}
	return terminal.Stdin()
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

// --- folded from suspend.go ---

// Suspend opens an exclusive-TTY window (Confirm's prompt/answer, an external
// interactive program) around fn. It clears the live region and keeps every
// paint off the surface until fn returns, so a sibling's frame cannot
// overwrite the window; it does not hold the scheduler, so eligible siblings
// keep running (contract §3) and their rows paint once the window closes. It
// is unexported by contract: a stopped spinner is not a product state. Child
// processes write to task.Writer() so the live row keeps moving.
func (o *Run) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	window := o.openLiveWindow()

	err := fn()

	o.closeLiveWindow(window)
	return err
}

// liveWindow is one Suspend's hold on the live region: whether it quiesced
// the region at all (an interactive surface), and whether that cleared a
// painted frame the close must bring back.
type liveWindow struct {
	opened  bool
	cleared bool
}

// openLiveWindow quiesces the live region: no paint reaches the surface
// until every open window closes.
func (o *Run) openLiveWindow() liveWindow {
	o.mu.Lock()
	defer o.mu.Unlock()
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return liveWindow{}
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
	}
	o.live.quiesced++
	return liveWindow{opened: true, cleared: o.live.clear(live)}
}

// closeLiveWindow ends w. Once the last window closes, the region repaints:
// the frame w cleared, or the rows siblings started inside it. The durable
// text siblings settled inside the windows prints first, above the region.
func (o *Run) closeLiveWindow(w liveWindow) {
	if !w.opened {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.live == nil {
		return
	}
	o.live.quiesced--
	if o.live.quiesced > 0 {
		return
	}
	o.flushHeldTextLocked()
	switch {
	case w.cleared && o.needsSpinnerAnimLocked():
		o.live.visible = true
		o.renderLiveLocked(true)
	case o.hasLiveActivityLocked():
		o.signalLiveLocked(true)
	}
}

func NonInteractive(argv ...string) ConfirmOption {
	if len(argv) == 0 {
		return policyFlag("--yes")
	}
	return policyFlag(argv[0])
}
