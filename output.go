package evo

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/clock"
	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/misuse"
	"github.com/zachbornheimer/evident-output/internal/process"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/live"
	renderplain "github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// Output is the aggregate root for one command's presentation lifecycle.
type Run struct {
	// mu guards the render state. Its sections hold the run's notifications,
	// so the engine's listener hears a section's writes once the mutex is free.
	mu  record.Mutex
	cfg config

	outputID string
	// startedAt is the wire v2 envelope's run_id-adjacent "started_at"
	// (spec §35) — captured once at construction through the Clock facade,
	// read back (never re-derived) at Finish.
	startedAt time.Time
	version   uint64
	// pulled and settles are followRecordLocked's scratch: the Tasks the
	// record reported changed, and the ones among them that settled.
	pulled  []record.TaskID
	settles []settleReaction
	// following is set while followRecordLocked runs, so an event it appends
	// does not follow the record again.
	following bool
	closed    bool
	// closing is non-nil once a Close call claimed the teardown; it closes
	// when that teardown ends, so a concurrent Close waits instead of
	// tearing down twice.
	closing chan struct{}
	// confirmTurn holds one token while a Confirm owns the terminal (see
	// awaitConfirmTurn); nil until the first Confirm asks.
	confirmTurn chan struct{}
	finishing   bool
	finished    bool
	armed       bool // set by arm(): live surface may paint before any entity exists
	// misuse records the first API misuse of this run. Graph workers call it
	// without Output.mu; a strict recorder panics after recording, never
	// under graph.mu.
	misuse *misuse.Recorder
	live   *liveEngine

	// changes performs the File, Exec, Patch and Effect operations of this
	// Run, and owns the workspace relative paths resolve against.
	changes *change.Run

	tasks []*taskState
	// rootTasks are the tasks outside every collection, in declaration
	// order: the ones a live frame walks beside the collections.
	rootTasks   []*taskState
	collections []*tasksState
	// rec holds the run's truth: its event journal, ledger, history lines,
	// messages, diagnostics, run-scoped facts and warnings, and Conclusion.
	rec *record.Run

	// wireEventErr latches the first §38 "evo.event" JSONL write failure
	// (structured_events.go's emitWireEventLocked).
	wireEventErr error
	// wireSeq is the last seq the evo.event stream numbered: the stream's own
	// counter, so its lines run 1, 2, 3 ... with no gap a consumer could
	// mistake for a lost line.
	wireSeq uint64

	// graph is the declared shape of the run: identity, keys and numbering.
	graph *graph.Graph
	// taskStates and containerStates are engine's own state for each node the
	// graph declared, keyed by the node's ID.
	taskStates      map[string]*taskState
	containerStates map[string]*tasksState
	// rootColumn is the alignment width for root Task rows.
	rootColumn rootColumn
	// Progressive durable emission (§17.5: terminal outcomes render immediately).
	// Finish only appends residual (unemitted entities + conclusion).
	linesEmitted int

	// durableRowsEmitted counts writeDurableTextLocked calls with non-empty
	// text — the one shared choke point every durable line goes through.
	// DeclareDryRun (I8) uses it to bound itself: once any row has
	// streamed, flipping DryRun retroactively would leave earlier rows not
	// reflecting it, so DeclareDryRun after that point is misuse.
	durableRowsEmitted int

	// debugPaneActive is true once a Debug record was subject to pane
	// presentation while interactive — either projected onto the live
	// rolling pane, or (dual-stream construction) routed to Diagnostics only
	// while pane presentation was still in effect. Controls failure
	// diagnostic-tail eligibility; not "the pane rendered this record".
	debugPaneActive bool

	// Print/Printf/Println line buffer.
	pendingPrint strings.Builder
	pendingVis   visibility // visibility of the current pending fragment

	// manifest is this Run's lazily opened reconciliation manifest (spec
	// §11.3): the first evo.File call opens it, and a Run that never uses one
	// never creates a manifest file.
	manifest *freshness.ManifestSession
	// taskFreshness remembers each Task's declared inputs, observed Basis,
	// pending operation records and Verify checks.
	taskFreshness *freshness.TaskTable
	// cleanStops tracks running Tasks that end Succeeded on a named signal.
	cleanStops cleanStopRegistry
}

func newOutput(subject string, options ...option) *Run {
	cfg := config{
		subject:         subject,
		clock:           systemClock{},
		visibilityDelay: defaultVisibilityDelay,
		maxFrameRate:    defaultMaxFrameRate,
		width:           defaultWidth,
		debugLevel:      levelInfo,
		redactor:        process.NoopRedactor{},
		maxEntities:     defaultMaxEntities,
		verbosity:       verbosityNormal,
		processRunner:   process.System(),
		fileFS:          osFileFS{},
	}
	for _, opt := range options {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	// A Terminal driver supplied without to() must still land its
	// non-interactive/residual projection somewhere (release-gate round 8
	// finding 2): default primary to the driver's own Sink() when it
	// reports one. A driver that explicitly reports no fixed sink (nil —
	// e.g. a virtual test screen driving output straight through the live
	// surface) is left alone; a driver that cannot answer the question at
	// all is misuse, recorded below once o exists.
	//
	// Whenever the driver's sink IS the primary writer — whether that's this
	// same defaulting, or an explicit to()/withDiagnostics() (Options path) or
	// Config's own to(c.Facades.Stdout)/to(c.Facades.Stderr) wiring (Config.Terminal path,
	// via configToOptions) that happens to coincide — the driver already
	// rendered the conclusion there once; Finish's dual-write branch must
	// not render it there again (release-gate round 9 finding 1).
	terminalWithoutSink := false
	if cfg.terminal != nil {
		sr, ok := cfg.terminal.(sinkReporter)
		switch {
		case !ok:
			terminalWithoutSink = cfg.primary == nil
		case sr.Sink() == nil:
			// Explicit "no fixed sink" — left alone.
		case cfg.primary == nil:
			cfg.primary = sr.Sink()
			cfg.samePrimaryAsTerminal = true
		case sr.Sink() == cfg.primary:
			cfg.samePrimaryAsTerminal = true
		case sr.Sink() == cfg.diagnostic:
			// Driver owns the diagnostic stream; primary is a distinct
			// stream the caller separately configured — both streams get
			// their own copy by design, not a duplicate.
		}
	}
	// An Options build that installs neither to() nor a Terminal sink must
	// still land its residual/conclusion projection somewhere — before this,
	// primary stayed nil and Finish silently wrote zero bytes, even on a
	// Fail (exit 2 with no evidence of why). Default to os.Stdout, matching
	// the non-Options default, and apply the same TTY/color inference the
	// non-Options path applies to that stream: never override an explicit
	// withNoColor(), only ever strengthen it, so a defaulted destination piped
	// to a file never leaks raw ANSI into it (release-gate round 9 findings
	// 2 and 5).
	if cfg.terminal == nil && cfg.primary == nil {
		cfg.primary = terminal.Stdout()
		if !cfg.noColor && (lookupEnv(envKeyNoColor) != "" || !writerIsCharDevice(cfg.primary)) {
			cfg.noColor = true
		}
	}
	if cfg.maxEntities <= 0 {
		cfg.maxEntities = defaultMaxEntities
	}
	if cfg.maxEvents <= 0 {
		cfg.maxEvents = defaultMaxEvents
	}
	resolveGlyphProfileLocked(&cfg)
	run := record.NewRun()
	o := &Run{
		cfg:             cfg,
		rec:             run,
		misuse:          misuse.New(cfg.strict),
		outputID:        "out_1",
		taskStates:      make(map[string]*taskState),
		containerStates: make(map[string]*tasksState),
	}
	o.manifest = freshness.NewManifestSession(o.manifestConfig)
	o.taskFreshness = freshness.NewTaskTable()
	o.graph = graph.New(run, graph.WithMaxEntities(cfg.maxEntities), graph.WithMaxConcurrency(cfg.maxConcurrency), graph.WithMisuseSink(o.misuse))
	o.changes = change.NewRun(changeHost{out: o}, o.graph)
	o.mu.Bind(run)
	run.SetListener(outputListener{o: o})
	// Stable-enough id for a process-local output instance.
	o.outputID = o.graph.NextID("out")
	o.startedAt = o.cfg.clock.Now()
	o.appendEventLocked(recEvent{Type: "output.started", OutputID: o.outputID})
	o.emitWireEventLocked(wire.EventRunStarted, "", runStartedPayload(o.cfg.subject))
	if terminalWithoutSink {
		o.recordMisuse(errTerminalWithoutSink)
	}
	if cfg.dryRun {
		// Announce before any task/item can reach the durable stream: no
		// caller path can finish a DryRun-configured Output without this
		// line having appeared first (evo-rec.md Problem 1). Safe to write
		// unlocked — o has not yet been returned to the caller.
		o.emitPlannedHeaderLocked()
	}
	return o
}

// emitPlannedHeaderLocked writes the planned run's opening announcement
// immediately, once, through the same durable-write path
// (writeDurableTextLocked) every other library-owned line uses.
func (o *Run) emitPlannedHeaderLocked() {
	var b strings.Builder
	renderplain.WritePlannedHeader(&b, !o.cfg.noColor, o.cfg.preview, o.cfg.dryRunHeaderText)
	o.writeDurableTextLocked(b.String())
}

// attachVerificationLocked appends details onto taskID's own running
// record — evo.File/evo.Exec's per-attribute reconciliation evidence
// (spec §2/§8.2), recorded before the task resolves so it is already
// present by the time Fail's terminal verb reads the task's state
// (the same "annotate before terminal" timing Fact and warning-severity Problem require). A no-op
// once the task has already resolved or does not exist.
func (o *Run) attachVerificationLocked(taskID string, details []record.VerificationDetail) {
	st := o.taskStates[taskID]
	if st == nil || record.IsTerminalTask(st.node.Rec.State()) {
		return
	}
	stored := st.node.Rec.AttachVerification(details)
	st.markFiling()
	// Emit the sanitized copy evo.run projects, so JSONL and JSON agree.
	for _, d := range stored {
		o.emitWireEventLocked(wire.EventVerificationObserved, taskID, wire.ToVerificationDoc(d).EventPayload())
	}
}

// promoteRunningLocked transitions a Pending task to Running on its first
// unit of evidence (Phase/Progress/Advance/Bytes/Step/Writer
// write). A work callback starting is announced by taskStarted, since the
// scheduler's claim already moved the row.
// For a sequential collection (Sequence), it records misuse when a sibling is
// already Running, enforcing the heart contract "one Running child"
// (evo-rec.md) — callers still get the transition; Strict mode is what
// escalates the violation to a panic. A plain Group collection
// documents its children as independent (worker-pool fan-out is a
// supported, concurrency-safe pattern there), so it is not policed.
func (o *Run) promoteRunningLocked(st *taskState) {
	st.node.Rec.Transition(running)
	o.announceRunningLocked(st)
}

// announceRunningLocked is the render state a Task owes for becoming Running,
// whoever moved its row: the Sequence check, the filing, the plain heartbeat
// and the task.started event. It answers once per Task.
func (o *Run) announceRunningLocked(st *taskState) {
	if st.runningAnnounced {
		return
	}
	st.runningAnnounced = true
	st.timing.Started = o.cfg.clock.Now()
	if col := st.collection(); col != nil && col.node.Sequential {
		col.runningSteps = slices.DeleteFunc(col.runningSteps, func(s *taskState) bool { return s.node.Rec.State() != running })
		if len(col.runningSteps) > 0 {
			o.recordMisuse(errConcurrentRunning)
		}
		col.runningSteps = append(col.runningSteps, st)
	}
	st.markFiling()
	o.armPlainHeartbeatLocked(st, o.cfg.clock.Now())
	o.emitWireEventLocked(wire.EventTaskStarted, st.id, nil)
}

func (o *Run) bumpLocked() {
	o.version++
}

// --- folded from debug.go ---

// DebugPresentation selects how structured debug records project to a TTY (§4.6 / §21.3).
type debugPresentation int

const (
	// DebugPresentationHistory appends durable scrollback above the live region (default).
	debugPresentationHistory debugPresentation = iota
	// DebugPresentationPane keeps a bounded rolling viewport inside the live region.
	debugPresentationPane
)

const (
	defaultDebugPaneHeight = 5
	debugPaneHeadingNewest = "── debug · newest first ────────────────────────────────────────"
	debugPaneHeadingOldest = "── debug · oldest first ────────────────────────────────────────"
	debugTailHeading       = "── diagnostics ─────────────────────────────────────────────────"
)

// DebugPaneOption configures DebugPane presentation.
type debugPaneOption interface {
	applyPane(*debugPaneConfig)
}

type debugPaneConfig struct {
	height      int
	newestFirst bool
	// preserveAlways forces a diagnostic tail on every Finish (demo / explicit policy).
	preserveAlways bool
	// preserveOnBad defaults true for pane mode: append tail on failed/blocked/cancelled.
	preserveOnBad bool
}

// rows is how many records the pane shows: its configured height, or the
// default when none was set.
func (c debugPaneConfig) rows() int {
	if c.height <= 0 {
		return defaultDebugPaneHeight
	}
	return c.height
}

type paneOptionFunc func(*debugPaneConfig)

func (f paneOptionFunc) applyPane(c *debugPaneConfig) { f(c) }

// PaneHeight sets how many debug records are visible in the pane (not including heading).
func paneHeight(lines int) debugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) {
		if lines > 0 {
			c.height = lines
		}
	})
}

// NewestFirst orders the pane with the most recent record first (default).
func newestFirst() debugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.newestFirst = true })
}

// OldestFirst orders the pane chronologically (oldest visible first).
func oldestFirst() debugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.newestFirst = false })
}

// PreserveDebugTail always emits a bounded diagnostic tail under the final report.
// Without this, pane mode still preserves a tail on failed/blocked/cancelled conclusions.
func preserveDebugTail() debugPaneOption {
	return paneOptionFunc(func(c *debugPaneConfig) { c.preserveAlways = true })
}

// DebugHistory selects durable append-above-and-redraw presentation (v0.4 default).
func debugHistory() option {
	return optionFunc(func(c *config) {
		c.debugPresentation = debugPresentationHistory
	})
}

// DebugPane selects a rolling TTY debug viewport at the bottom of the live region.
func debugPane(opts ...debugPaneOption) option {
	return optionFunc(func(c *config) {
		c.debugPresentation = debugPresentationPane
		cfg := debugPaneConfig{
			height:        defaultDebugPaneHeight,
			newestFirst:   true,
			preserveOnBad: true,
		}
		for _, o := range opts {
			if o != nil {
				o.applyPane(&cfg)
			}
		}
		c.debugPane = cfg
	})
}

// debugRecord is one structured diagnostic journal entry (§21.3).
type debugRecord = record.DebugRecord

// formatHistoryLine is the compact bracketed grammar for durable scrollback.
// Example: 12:04:18.219 [DEBUG] package index loaded  packages=18
func formatHistoryLine(rec debugRecord, color bool) string {
	levelTok := "[" + rec.Level + "]"
	if color {
		// Spec: color applies primarily to the level token. Warn/Error reuse
		// the same entity glyph palette a Task's own Warning/Failed row uses
		// (stateColor) — one severity vocabulary, not a second one invented
		// for the debug journal (release-gate round 9 finding 3). Debug/Info
		// keep the journal's existing cyan.
		levelTok = txt.ApplySGR(levelTok, debugLevelColor(rec.Level), true)
	}
	msg := txt.Text(rec.Message)
	var body string
	if rec.Time.IsZero() {
		body = levelTok + " " + msg
	} else {
		body = rec.Time.Format("15:04:05.000") + " " + levelTok + " " + msg
	}
	if attrs := formatHistoryAttrs(rec.Fields); attrs != "" {
		body += "  " + attrs
	}
	return body
}

// debugLevelColor maps a journal record's level name to the SGR color its
// level token renders with — LevelWarn and LevelError reuse stateColor's
// Warning/Failed colors (txt.SGRYellow/txt.SGRRed); every other level (Debug, Info,
// Trace) keeps the journal's existing cyan.
func debugLevelColor(level string) string {
	switch level {
	case "WARN":
		return txt.SGRYellow
	case "ERROR":
		return txt.SGRRed
	default:
		return txt.SGRCyan
	}
}

func formatHistoryAttrs(fields []field) string {
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		val := fmt.Sprint(f.Value)
		if f.Sensitive {
			val = record.RedactedValue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", txt.Text(f.Key), txt.Text(val)))
	}
	return strings.Join(parts, " ")
}

// formatLivePaneLine renders rec for the rolling live pane, narrowing to fit
// columns. One clock, one grammar (release-gate round 6 finding 3): every
// human debug surface — history, live pane, diagnostics tail — renders the
// same bracketed local-time grammar as formatHistoryLine; only the machine
// LogRecord (slog.Record.PC, structured Fields) carries anything else.
// color is always false here — the caller wraps the whole composed line in
// txt.Dim() itself, so the level token is never colored underneath that wrap.
func formatLivePaneLine(rec debugRecord, columns int) string {
	full := formatHistoryLine(rec, false)
	if txt.VisibleCells(full) <= columns {
		return full
	}
	// Too narrow for the full line (attrs included): truncate to the column
	// budget with a trailing "…" rather than silently dropping Fields — the
	// pane must never claim a record had no attributes when it simply ran
	// out of room to show them.
	return txt.Truncate(full, columns)
}

// debugPaneReservedRows is the number of terminal rows the debug pane needs
// (heading + up to height records), used to budget the live task body.
func debugPaneReservedRows(pane debugPaneConfig, recordCount int) int {
	if recordCount <= 0 {
		return 0
	}
	height := min(recordCount, pane.rows())
	// blank separator line before heading is written by writeDebugPane's leading \n
	// on body that may already end without newline — count heading + records + 1.
	return height + 2
}

// writeDebugPane appends the rolling pane section to a live-region builder.
func writeDebugPane(b *strings.Builder, records []debugRecord, pane debugPaneConfig, columns int, color bool) {
	if len(records) == 0 {
		return
	}
	height := pane.rows()
	heading := debugPaneHeadingNewest
	if !pane.newestFirst {
		heading = debugPaneHeadingOldest
	}
	if color {
		heading = txt.Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')

	view := paneView(records, height, pane.newestFirst)
	for _, rec := range view {
		line := formatLivePaneLine(rec, columns)
		if color {
			line = txt.Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

// paneView returns up to height records in display order.
func paneView(records []debugRecord, height int, newestFirst bool) []debugRecord {
	if height <= 0 || len(records) == 0 {
		return nil
	}
	n := len(records)
	start := 0
	if n > height {
		// Both orderings show the tail window; newestFirst only changes display order (below).
		start = n - height
	}
	slice := records[start:]
	if !newestFirst {
		return slice
	}
	// newest first: reverse the visible window
	out := make([]debugRecord, len(slice))
	for i := range slice {
		out[i] = slice[len(slice)-1-i]
	}
	return out
}

// writeDebugTail appends a labeled diagnostic tail (slog text) after the final report.
func writeDebugTail(b *strings.Builder, records []debugRecord, max int, color bool) {
	if len(records) == 0 {
		return
	}
	if max <= 0 {
		max = defaultDebugPaneHeight
	}
	heading := debugTailHeading
	if color {
		heading = txt.Dim(heading, true)
	}
	b.WriteByte('\n')
	b.WriteString(heading)
	b.WriteByte('\n')
	view := paneView(records, max, true) // newest first in failure tail
	for _, rec := range view {
		line := formatHistoryLine(rec, false)
		if color {
			line = txt.Dim(line, true)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func (o *Run) shouldPreserveDebugTailLocked(conc Conclusion) bool {
	// Explicit opt-in always wins (including plain demos with PreserveDebugTail).
	if o.cfg.debugPane.preserveAlways {
		return o.cfg.debugPresentation == debugPresentationPane && o.rec.DebugRecordCount() > 0
	}
	// Default failure tail only when the rolling pane actually owned presentation.
	// History fallback (plain/non-TTY) already streamed durable lines — no second dump.
	if !o.debugPaneActive || o.cfg.debugPresentation != debugPresentationPane {
		return false
	}
	if !o.cfg.debugPane.preserveOnBad {
		return false
	}
	switch conc.Outcome {
	case Failed, Refused, Cancelled:
		return true
	default:
		return false
	}
}

// --- folded from debug_emit.go ---

// Debug records a structured diagnostic (§4.6 / §21.3).
//
// History mode (default): durable scrollback above the live region (or plain stream).
// Pane mode: record is journaled and shown in the rolling live pane; not durable
// scrollback unless a diagnostic tail is preserved at Finish.
//
// When Diagnostics is configured and is a different writer than the primary stream,
// debug lines go to Diagnostics only (not the human Items/Tasks stream). Use
// Capture for child-process evidence instead of DebugWriter when you need Fail Detail.
func (o *Run) debug(message string, fields ...field) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitDebugLocked(message, fields, false)
}

func (o *Run) emitDebugLocked(message string, fields []field, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > levelDebug {
		return
	}
	rec := o.newDebugRecordLocked("DEBUG", message, fields, time.Time{})
	o.projectDebugRecordLocked(rec)
}

// emitDebugRecordLocked journals a fully-formed debug record (slog bridge).
// force bypasses DebugLevel filtering after slog already applied its min level.
func (o *Run) emitDebugRecordLocked(levelName, message string, fields []field, at time.Time, force bool) {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	if !force && o.cfg.debugLevel > levelDebug {
		return
	}
	rec := o.newDebugRecordLocked(levelName, message, fields, at)
	o.projectDebugRecordLocked(rec)
}

func (o *Run) newDebugRecordLocked(levelName, message string, fields []field, at time.Time) debugRecord {
	if at.IsZero() {
		at = o.cfg.clock.Now()
	}
	rec := debugRecord{
		Time:    at,
		Level:   levelName,
		Message: txt.Text(message),
		Fields:  cloneFields(fields),
	}
	for i := range rec.Fields {
		if rec.Fields[i].Sensitive {
			rec.Fields[i].Value = record.RedactedValue
		} else if o.cfg.redactor != nil {
			rec.Fields[i].Value = o.cfg.redactor.RedactString(fmt.Sprint(rec.Fields[i].Value))
		}
	}
	return rec
}

func (o *Run) projectDebugRecordLocked(rec debugRecord) {
	o.rec.AppendDebugRecord(rec)
	history := formatHistoryLine(rec, !o.cfg.noColor)
	plainHistory := formatHistoryLine(rec, false)
	o.rec.AppendLine(plainHistory)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "log.emitted"})

	dual := o.cfg.diagnostic != nil && o.cfg.primary != nil && o.cfg.diagnostic != o.cfg.primary
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	panePresentation := o.cfg.debugPresentation == debugPresentationPane

	// dualSameTerminal is true when Diagnostics is a distinct writer from
	// primary but both resolve to the SAME physical terminal (the realistic
	// default: Config{Stdout, Stderr} on an interactive shell, where fd 1 and
	// fd 2 name one controlling tty). Writing raw bytes straight to
	// Diagnostics in that case bypasses the live region's clear-live ->
	// write-durable -> repaint sequencing and corrupts the spinner row on
	// the one screen both writers share — route it through the same
	// sequencing single-stream records use instead.
	dualSameTerminal := dual && interactive && o.cfg.diagnosticSharesTerminal

	if o.cfg.diagnostic != nil && !dualSameTerminal {
		o.writeDiagnosticTextLocked(plainHistory + "\n")
	}
	if dual && !dualSameTerminal {
		// The record already went to Diagnostics above; don't also duplicate
		// it onto the live pane/durable primary. Still mark pane presentation
		// as active so a failing Finish can preserve the diagnostics tail on
		// the live terminal (§21.3.2) — otherwise the promised failure tail
		// is unreachable whenever the caller uses two distinct real streams
		// (the realistic default: Config{Stdout, Stderr} routes to(Stdout),
		// withDiagnostics(Stderr)).
		if interactive && panePresentation {
			o.debugPaneActive = true
		}
		o.linesEmitted = o.rec.LineCount()
		return
	}

	if interactive && panePresentation {
		o.debugPaneActive = true
		o.linesEmitted = o.rec.LineCount()
		o.signalLiveLocked(true)
		return
	}
	if interactive {
		o.debugLiveLocked(history)
	} else {
		o.writeDurableTextLocked(history + "\n")
	}
	o.linesEmitted = o.rec.LineCount()
}

// writeDiagnosticText emits text on the Diagnostics writer (thread-safe).
func (o *Run) writeDiagnosticText(text string) {
	if o == nil || text == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeDiagnosticTextLocked(text)
}

func (o *Run) writeDiagnosticTextLocked(text string) {
	if o.cfg.diagnostic == nil || text == "" {
		return
	}
	_, _ = io.WriteString(o.cfg.diagnostic, text)
	if f, ok := o.cfg.diagnostic.(flusher); ok {
		_ = f.Flush()
	}
}

func cloneFields(in []field) []field {
	if len(in) == 0 {
		return nil
	}
	out := make([]field, len(in))
	copy(out, in)
	return out
}

// --- folded from debug_writer.go ---

// --- folded from format.go ---

func init() {
	// Single source of truth: PublishedRelease, pinned once into the
	// v2 wire encoder path (spec §32.1/§35's evo_version).
	setWireEvoVersion(publishedRelease)
}

// PublishedRelease is the single source of truth for the current published
// module and MCP pin used in install guidance.
//
// When cutting a release:
//  1. Promote CHANGELOG ## Unreleased → ## [X.Y.Z] (Keep a Changelog).
//  2. Set publishedRelease to the new tag (e.g. "v1.0.1").
//  3. Prefer: mise run test && mise run cut-release
//  4. Tag that commit; do not move prior tags.
//
// version_drift_test.go enforces the portable surface stays synchronized.
const publishedRelease = "v2.0.0"

// --- folded from glyph_resolve.go ---

// Glyphs selects the glyph capability profile (default GlyphsAuto).
func glyphs(p glyphProfile) option {
	return optionFunc(func(c *config) { c.glyphs = p })
}

// resolveGlyphProfileLocked turns a possibly-auto profile into a concrete
// one. Called once at construction (newOutput) so every render call reads a
// decided value instead of re-detecting locale per frame.
func resolveGlyphProfileLocked(cfg *config) {
	if cfg.glyphs != glyphsAuto {
		return
	}
	if !interactiveOutputLocked(cfg) || localeAdvertisesUTF8() {
		cfg.glyphs = glyphsUnicode
		return
	}
	cfg.glyphs = glyphsASCII
}

// interactiveOutputLocked reports whether the configured terminal is a real
// interactive surface. Plain/non-interactive projection keeps the Unicode
// vocabulary unconditionally — evo-rec.md's glyph-safety default only
// guards the live status column a human is watching.
func interactiveOutputLocked(cfg *config) bool {
	if cfg.plain {
		return false
	}
	ls := asLive(cfg.terminal)
	return ls != nil && ls.IsInteractive()
}

// localeAdvertisesUTF8 checks LC_ALL, then LC_CTYPE, then LANG (POSIX
// override order) for a UTF-8 marker. Read directly at construction time —
// the same pattern Config.Color's NO_COLOR detection uses — so no render
// path re-reads the environment.
func localeAdvertisesUTF8() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := process.Env(name); v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "utf8")
		}
	}
	// No locale env set at all: assume the historical default (UTF-8) rather
	// than guessing ASCII from absence.
	return true
}

// --- folded from journal.go ---

// appendEventLocked journals e. The journal is in the order things happened,
// so the record is followed first: a settle a graph goroutine made on its own
// is journaled before an event this section writes after it, however late the
// record's listener hears it.
func (o *Run) appendEventLocked(e recEvent) {
	o.followRecordLocked()
	e.Timestamp = o.cfg.clock.Now()
	e.SchemaVersion = eventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = o.outputID
	}
	e = o.rec.AppendEvent(e, o.cfg.maxEvents)
	if o.cfg.projection == projectionStreamJSON {
		o.writeStreamJSONLocked(e)
	}
}

func (o *Run) writeStreamJSONLocked(e recEvent) {
	w := o.cfg.primary
	if w == nil {
		return
	}
	row, err := wire.EncodeEventJSON(e)
	if err != nil {
		return
	}
	_, _ = w.Write(row)
	_, _ = w.Write([]byte{'\n'})
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}

// --- folded from ledger_entry.go ---

// ledgerTarget is where a Task's ledger rows go: the Task that owns the
// section, and the tense the run records in.
type ledgerTarget struct {
	owner *taskState
	tense ledgerTense
}

// resolveLedgerTarget resolves the task named by taskID to its ledger
// target — the shared guard (open, not yet resolved) behind every change a
// Task records. err is non-nil (already recorded as misuse where the
// cause is not simply "the task no longer exists") when the caller should
// record nothing further.
func (o *Run) resolveLedgerTarget(taskID string) (ledgerTarget, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil {
		return ledgerTarget{}, errClosed
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return ledgerTarget{}, err
	}
	if record.IsTerminalTask(st.node.Rec.State()) {
		o.recordMisuseFor(st.name, errAlreadyResolved)
		return ledgerTarget{}, errAlreadyResolved
	}
	return ledgerTarget{owner: st, tense: record.TenseFor(o.cfg.dryRun)}, nil
}

// recordResolvedEntry records e into target's section, conjugating the verb
// to past tense for the applied ledger only. It takes an already-resolved
// target: Effect resolves once, before running its callback, because
// re-resolving afterward would re-open the terminal-task check to a state a
// concurrent resolution may have legitimately changed, dropping a real
// effect as spurious misuse. The imperative verb is kept as the section's
// intended verb, so a section that ends up with zero rows renders "nothing
// to <verb> <subject>" (evo-rec.md Problem 18).
func (o *Run) recordResolvedEntry(taskID string, target ledgerTarget, e record.LedgerEntry) {
	verb, event := e.Verb(), wire.EventEffectPlanned
	if target.tense == tenseChanged {
		verb, event = txt.ConjugatePast(e.Verb()), wire.EventEffectCommitted
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	sectionID := o.ledgerSectionIDLocked(target.owner, target.tense)
	if o.rec.RecordEntry(target.owner.id, target.tense, verb, e) {
		o.bumpLocked()
		o.appendEventLocked(recEvent{Type: target.tense.RecordedEvent(), EntityID: sectionID})
	}
	o.emitWireEventLocked(event, taskID, e.Payload(verb))
}

// --- folded from ledger_order.go ---

// appendTaskLocked adds st to the run's Tasks in declaration order. Caller
// must hold o.mu.
func (o *Run) appendTaskLocked(st *taskState) {
	o.tasks = append(o.tasks, st)
	if st.collection() == nil {
		o.rootTasks = append(o.rootTasks, st)
		o.rootColumn.add(st.name)
	}
}

// heldBackAsNoOpLocked reports whether a resolved root Task is a
// zero-information row (txt.IsProvenNoOpTask) with no ledger section
// of its own. Such a row is not committed when it resolves: Finish decides
// whether the run has anything else to show, and prints the row only if not.
// Caller must hold o.mu.
func (o *Run) heldBackAsNoOpLocked(t taskSnapshot) bool {
	if !txt.IsProvenNoOpTask(txt.TaskAtVerbosity(t, o.cfg.verbosity >= verbosityVerbose)) {
		return false
	}
	return !o.rec.HasLedgerSection(t.ID)
}

// --- folded from ledger_section.go ---

// ledgerTense is which ledger a section belongs to: [changed] records work
// that happened, [planned] work a dry run or preview would do.
type ledgerTense = record.LedgerTense

const (
	tenseChanged = record.TenseChanged
	tensePlanned = record.TensePlanned
)

// ledgerOwner is st as the ledger knows its Task: placed by declaration,
// found by id, shown by name, qualified by container path.
func (st *taskState) ledgerOwner() record.LedgerOwner {
	return record.LedgerOwner{ID: st.id, Name: st.name, Declaration: st.node.Declaration, Containers: st.containerPath()}
}

// ledgerSectionIDLocked returns the id of owner's section in tense, opening
// it on first use. Caller must hold o.mu.
func (o *Run) ledgerSectionIDLocked(owner *taskState, tense ledgerTense) string {
	if id, ok := o.rec.SectionID(owner.id, tense); ok {
		return id
	}
	id := o.rec.OpenSection(owner.ledgerOwner(), tense, o.graph.NextID(tense.String()))
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: tense.DeclaredEvent(), EntityID: id})
	return id
}

// hasRecordedEffectLocked reports whether the Task taskID's sections carry
// at least one row — see the unresolved-task amnesty in Finish (beginner-1,
// I1). Caller must hold o.mu.
func (o *Run) hasRecordedEffectLocked(taskID string) bool {
	return o.rec.HasRecordedEffect(taskID)
}

// hasPlannedEffect reports whether the Task taskID recorded at least one
// [planned] row: a mutation a dry run or preview skipped.
func (o *Run) hasPlannedEffect(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.rec.HasPlannedEffect(taskID)
}

// --- folded from listener.go ---

// declaredState is the state a Task is declared in, and so the state the
// render state first follows it from.
const declaredState = pending

// The engine's render state follows the record by pulling, never by being
// pushed. A write to the record logs its Task while the record's lock is
// held (record.Run.TakeChanged), so anything that reads the live index first
// pulls that log and finds every write the record has seen, whichever
// goroutine made it and whether or not the record has told its listener yet.
// The listener is only a prompt to pull: the engine hears a change made
// outside Output.mu, takes the mutex itself, and follows the record. No
// handler needs to run inside a writer's lock.

// outputListener is how the engine hears a change made by any writer.
type outputListener struct{ o *Run }

// TaskChanged has nothing to do: a change waits in the log for the next
// read of the index.
func (outputListener) TaskChanged(record.TaskID, record.State, record.State) {}

// TaskSettled follows the record, so the settle is answered promptly even
// when nothing reads the index. The record names the settle itself, so the
// engine cannot disagree with the writer about which states end a Task.
func (l outputListener) TaskSettled(record.TaskID, record.State, record.State) {
	l.o.mu.Lock()
	defer l.o.mu.Unlock()
	l.o.followRecordLocked()
}

// settleReaction is a Task that settled and the state it settled into.
type settleReaction struct {
	st *taskState
	to entityState
}

// followRecordLocked brings the render state up to date with the record:
// the live index first, then what each terminal transition owes. A call made while
// it is already running does nothing. Callers hold o.mu.
func (o *Run) followRecordLocked() {
	if o.following {
		return
	}
	o.following = true
	defer func() { o.following = false }()
	o.pullRecordLocked()
	o.reactToSettlesLocked()
}

// pullRecordLocked brings the live index (census and filing) up to date with
// every write the record has seen, and notes the Tasks that settled so
// reactToSettlesLocked can answer each once. It changes nothing else, so a
// read of the index may pull in the middle of building a frame. Callers hold
// o.mu.
func (o *Run) pullRecordLocked() {
	o.pulled = o.rec.TakeChanged(o.pulled[:0])
	for _, id := range o.pulled {
		st := o.taskStates[string(id)]
		if st == nil {
			continue
		}
		st.markFiling()
		st.censusSync()
		if st.node.Rec.IsSettled() && !st.settleAnswered {
			st.settleAnswered = true
			o.settles = append(o.settles, settleReaction{st: st, to: st.followed})
		}
	}
}

// reactToSettlesLocked answers each Task pullRecordLocked saw settle. Callers
// hold o.mu.
func (o *Run) reactToSettlesLocked() {
	for i := range o.settles {
		o.taskSettledLocked(o.settles[i].st, o.settles[i].to)
		o.settles[i] = settleReaction{}
	}
	o.settles = o.settles[:0]
}

// taskSettledLocked is the render state a terminal transition owes, whichever
// code settled the Task: the live filing, the plain heartbeat, the snapshot
// version and the task.<state> event. The live census moved when the record
// was pulled. Callers hold o.mu.
func (o *Run) taskSettledLocked(st *taskState, to entityState) {
	st.timing.Settled = o.cfg.clock.Now()
	o.emitWireEventLocked(wire.EventTaskFinished, st.id, taskFinishedPayload(st))
	if st.isGate() {
		o.bumpLocked()
		return
	}
	st.markFiling()
	o.stopPlainHeartbeatLocked(st)
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task." + string(to), EntityID: st.id})
}

// --- folded from live.go ---

// LiveSurface is an interactive terminal sink for live-region rendering.
// testkit.Screen implements this; production drivers can as well.
type liveSurface interface {
	terminalDriver
	Columns() int
	Rows() int
	IsInteractive() bool
	WriteLive(text string)
	ClearLive()
	WriteDurable(line string)
	WriteFinal(text string)
}

// asLive returns a LiveSurface when the configured terminal supports it.
func asLive(d terminalDriver) liveSurface {
	if d == nil {
		return nil
	}
	if ls, ok := d.(liveSurface); ok {
		return ls
	}
	return nil
}

type liveEngine struct {
	surface liveSurface
	visible bool
	// quiesced counts the open Suspend windows (guarded by o.mu): while any
	// is open nothing is painted, and no signal makes the region visible.
	quiesced int
	// heldText is the durable text that settled while a window was open
	// (guarded by o.mu), in arrival order: the window owns the terminal, so
	// it prints once the last window closes.
	heldText []string

	// paintMu serializes writes to the surface and guards what describes
	// them. Lock order is o.mu, then paintMu; a paint never takes o.mu
	// while holding it.
	paintMu      sync.Mutex
	lastLiveText string
	liveActive   bool
	floorSeq     uint64 // frames at or below it were snapshotted before a clear
	paintedSeq   uint64 // the newest frame that reached paint

	// seq numbers frames in snapshot order; guarded by o.mu.
	seq uint64

	// lastRender is the domain time of the last write (nil: never painted),
	// lastWrite the wall time it began.
	lastRender    atomic.Pointer[time.Time]
	lastWrite     atomic.Pointer[time.Time]
	pendingRedraw atomic.Bool

	// meter and skippedGlyph back the animator's overload backpressure.
	meter        frameMeter
	skippedGlyph atomic.Int64

	// activitySince is set when live activity first appears; VisibilityDelay
	// withholds the first paint until the domain clock advances past the delay
	// (force terminal outcomes bypass the delay).
	activitySince time.Time
	waitingDelay  bool

	// anim drives independent spinner ticks while any task is Running,
	// so indeterminate rows animate even when determinate bars are idle.
	// Also used to promote visibility once VisibilityDelay elapses.
	animMu      sync.Mutex
	animRunning bool
	animStop    chan struct{}

	// resizeArmed is true when SIGWINCH watch is registered on the surface.
	resizeArmed bool

	// forced meters the render work forced paints spent in the current
	// frame interval.
	forced renderBudget
}

func (o *Run) liveLocked() liveSurface {
	if o.cfg.plain {
		return nil
	}
	return asLive(o.cfg.terminal)
}

// signalLiveLocked marks that interactive presentation may need a redraw.
// force=true bypasses frame-rate coalescing only (not VisibilityDelay).
// VisibilityDelay withholds the first live paint after activity starts, except
// a newly declared Running task always paints (FP-005: spinner before check).
func (o *Run) signalLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
		o.startResizeWatchLocked(live)
	}
	if o.live.quiesced > 0 {
		return
	}
	now := o.cfg.clock.Now()
	if !o.visibilitySettledLocked(now) && o.hasLiveActivityLocked() {
		if o.live.activitySince.IsZero() {
			o.live.activitySince = now
		}
		delay := o.cfg.visibilityDelay
		// delay <= 0 means immediate (tests use visibilityDelay(0)). A newly
		// declared Running task bypasses the delay so the first frame is a
		// spinner, not a popup-complete check (FP-005).
		if delay <= 0 || now.Sub(o.live.activitySince) >= delay || o.hasUnpaintedRunningLocked() || o.armedTitleLiveLocked() {
			o.live.visible = true
			o.live.waitingDelay = false
		} else {
			o.live.waitingDelay = true
			// Wall ticker re-checks delay; fixedClock tests re-enter after Advance.
			o.ensureSpinnerAnimatorLocked()
			return
		}
	}
	if !o.live.visible {
		return
	}
	lastRender := o.live.lastRender.Load()
	if lastRender != nil && len(o.tasks) > forcedRenderRowsPerInterval {
		// Past the forced budget's row count the animator owns painting: it
		// ticks every spinner period while anything is unresolved, so workers
		// never render inline. The first paint and every direct
		// renderLiveLocked caller (finish, suspend, durable redraw) stay
		// synchronous. The gate counts Tasks, not time, so it is deterministic.
		o.live.pendingRedraw.Store(true)
		o.ensureSpinnerAnimatorLocked()
		return
	}
	minGap := time.Second / time.Duration(max(1, o.cfg.maxFrameRate))
	if force && !o.live.forced.allow(now, minGap, len(o.tasks)) {
		force = false
	}
	if !force && lastRender != nil {
		if now.Sub(*lastRender) < minGap {
			// The animator paints the coalesced change on its next tick.
			o.live.pendingRedraw.Store(true)
			o.ensureSpinnerAnimatorLocked()
			return
		}
	}
	o.renderLiveLocked(force)
	o.ensureSpinnerAnimatorLocked()
}

// visibilitySettledLocked reports whether VisibilityDelay can no longer
// withhold a paint: the region is visible and the delay since activity
// began has elapsed. Past that point the activity scan decides nothing,
// and skipping it keeps each signal from walking every Task.
func (o *Run) visibilitySettledLocked(now time.Time) bool {
	return o.live.visible && !o.live.activitySince.IsZero() &&
		(o.cfg.visibilityDelay <= 0 || now.Sub(o.live.activitySince) >= o.cfg.visibilityDelay)
}

// holdRunningPaint keeps a Running row on screen for one spinner period so a
// fast bind cannot resolve in the same tick the spinner was painted — the
// viewer would otherwise see a popup-complete check (FP-005). Skipped when
// the clock is not the wall clock (tests inject Clock) or the surface is
// not an interactive live terminal.
func (o *Run) holdRunningPaint(id string) {
	o.mu.Lock()
	if _, ok := o.cfg.clock.(clock.Wall); !ok {
		o.mu.Unlock()
		return
	}
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		o.mu.Unlock()
		return
	}
	st := o.taskStates[id]
	if st == nil || st.node.Rec.State() != running {
		o.mu.Unlock()
		return
	}
	seen := st.liveFirstSeenAt
	o.mu.Unlock()
	wait := txt.SpinnerPeriod
	if !seen.IsZero() {
		wait = txt.SpinnerPeriod - wall.Since(seen)
	}
	if wait > 0 {
		wall.Sleep(wait)
	}
}

func (o *Run) hasUnpaintedRunningLocked() bool {
	for _, t := range o.tasks {
		if t.node.Rec.State() == running && t.liveFirstSeenAt.IsZero() {
			return true
		}
	}
	return false
}

func (o *Run) hasLiveActivityLocked() bool {
	for _, t := range o.tasks {
		// Pending counts too: a declared task renders its named "○" row
		// immediately (evo-rec.md "predeclare Tasks; ... others named
		// idle") — VisibilityDelay withholds the *spinner flash* for
		// near-instant work, not the fact that a task now exists.
		if t.node.Rec.State() == running || t.node.Rec.State() == pending || t.node.Rec.Phase() != "" {
			return true
		}
		if t.node.Rec.Progress().Kind == determinate || t.node.Rec.Progress().Kind == bytesKind {
			if t.node.Rec.State() == running || t.node.Rec.State().Succeeded() || t.node.Rec.State() == failed {
				return true
			}
		}
	}
	// Armed-but-empty: Init promised a paint before any entity exists. Once
	// the first Task/Tasks is declared, its own state drives activity.
	return o.armedTitleLiveLocked()
}

// armedTitleLiveLocked is the title-only live spinner painted by arm() before
// any Task exists. hasLiveActivityLocked and needsSpinnerAnimLocked must agree
// on this predicate — otherwise the header spinner is shown and then frozen.
func (o *Run) armedTitleLiveLocked() bool {
	return o.armed && len(o.tasks) == 0 && len(o.collections) == 0
}

// arm marks the live surface as ready to paint before any entity is declared,
// so Init honors the ≤100ms first-paint contract even when the caller does
// heavy work before the first Task. Idempotent.
func (o *Run) arm() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.armed {
		return
	}
	o.armed = true
	o.signalLiveLocked(false)
}

func (o *Run) renderLiveLocked(force bool) {
	live := o.liveLocked()
	if live == nil || o.live == nil || !o.live.visible || o.live.quiesced > 0 {
		return
	}
	o.paintFrameLocked(live)
}

// needsSpinnerAnimLocked reports whether any live row should keep ticking: the
// armed title-only spinner, or a Pending/Running task still rendered in the
// current frame — not a standalone task already flushed to durable text and
// dropped from the ticker (see liveSnapshotLocked's matching filter).
// Counting only Running here used to let an all-Pending frame freeze forever
// (evo-rec.md Problem 9). Omitting the armed title froze "⠦  zq" during
// Init-to-first-Task work (ensureReady, fingerprint, Inspect).
func (o *Run) needsSpinnerAnimLocked() bool {
	if o.armedTitleLiveLocked() {
		return true
	}
	for _, col := range o.collections {
		if census := col.currentCensus(); census.running+census.pending > 0 {
			return true
		}
	}
	for _, t := range o.rootTasks {
		if (t.node.Rec.State() == running || t.node.Rec.State() == pending) && !t.coreEmitted {
			return true
		}
	}
	return false
}

// ensureSpinnerAnimatorLocked starts a background tick that re-renders the live
// region so indeterminate spinners advance without waiting for Progress calls,
// and that promotes visibility once VisibilityDelay elapses.
func (o *Run) ensureSpinnerAnimatorLocked() {
	if o.live == nil || o.finished || o.closed {
		return
	}
	// A running animator re-checks needsSpinnerAnimLocked on every tick and
	// stops itself, so skip that O(n) scan on each signal while it runs.
	o.live.animMu.Lock()
	running := o.live.animRunning
	o.live.animMu.Unlock()
	if running {
		return
	}
	// Waiting for delay: keep a ticker so we can paint when the threshold elapses.
	switch {
	case o.live.waitingDelay:
		// fall through to start animator
	case !o.live.visible:
		return
	case !o.needsSpinnerAnimLocked() && !o.live.pendingRedraw.Load():
		o.stopSpinnerAnimatorLocked()
		return
	}
	o.live.animMu.Lock()
	if o.live.animRunning {
		o.live.animMu.Unlock()
		return
	}
	stop := make(chan struct{})
	o.live.animStop = stop
	o.live.animRunning = true
	o.live.animMu.Unlock()
	go o.spinnerAnimateLoop(stop)
}

func (o *Run) stopSpinnerAnimatorLocked() {
	if o.live == nil {
		return
	}
	o.live.animMu.Lock()
	if o.live.animRunning && o.live.animStop != nil {
		close(o.live.animStop)
		o.live.animStop = nil
		o.live.animRunning = false
	}
	o.live.animMu.Unlock()
}

// resizeWatcher is implemented by terminal.ANSI (SIGWINCH) and no-ops elsewhere.
type resizeWatcher interface {
	StartResizeWatch(onResize func())
	StopResizeWatch()
}

// startResizeWatchLocked arms SIGWINCH → RefreshSize + forced live redraw.
func (o *Run) startResizeWatchLocked(live liveSurface) {
	w, ok := live.(resizeWatcher)
	if !ok || o.live == nil || o.live.resizeArmed {
		return
	}
	o.live.resizeArmed = true
	w.StartResizeWatch(func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.closed || o.finished || o.live == nil || !o.live.visible {
			return
		}
		o.signalLiveLocked(true)
	})
}

func (o *Run) stopResizeWatchLocked() {
	if o.live == nil || !o.live.resizeArmed {
		return
	}
	if w, ok := o.live.surface.(resizeWatcher); ok {
		w.StopResizeWatch()
	}
	o.live.resizeArmed = false
}

func (o *Run) spinnerAnimateLoop(stop <-chan struct{}) {
	// Real wall timer: spinner cadence is independent of the domain clock and
	// of Progress/Phase call rate. Domain clock still selects the glyph frame
	// (fixedClock freezes animation for golden tests).
	t := wall.NewTimer(untilNextSpinnerSlot(wall.Now()))
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			t.Reset(untilNextSpinnerSlot(wall.Now()))
			o.mu.Lock()
			next := o.animatorTickLocked()
			if next == tickPaint || next == tickFlush {
				o.paintFrameUnlocked(o.live.surface)
			}
			o.mu.Unlock()
			if next == tickStop || next == tickFlush {
				return
			}
		}
	}
}

// animatorStep is what one animator tick decided.
type animatorStep int

const (
	tickContinue animatorStep = iota // nothing to paint; keep ticking
	tickStop                         // nothing to paint; the animator has stopped
	tickPaint                        // paint a frame, keep ticking
	tickFlush                        // paint the last coalesced change; the animator has stopped
)

// animatorTickLocked decides one tick: it settles visibility and stops the
// animator when nothing is left to animate, and skips a glyph-only repaint
// while an overloaded surface was written too recently to need it.
func (o *Run) animatorTickLocked() animatorStep {
	if o.closed || o.finished || o.live == nil {
		o.stopSpinnerAnimatorLocked()
		return tickStop
	}
	if o.live.quiesced > 0 {
		return tickContinue
	}
	// Promote visibility after VisibilityDelay using domain clock.
	if o.live.waitingDelay && o.hasLiveActivityLocked() {
		delay := o.cfg.visibilityDelay
		now := o.cfg.clock.Now()
		if delay <= 0 || now.Sub(o.live.activitySince) >= delay {
			o.live.visible = true
			o.live.waitingDelay = false
			o.renderLiveLocked(true)
		}
		return tickContinue
	}
	if !o.live.visible {
		o.stopSpinnerAnimatorLocked()
		return tickStop
	}
	if !o.needsSpinnerAnimLocked() {
		o.stopSpinnerAnimatorLocked()
		// Paint the last coalesced change before going quiet, so
		// the frame on screen is never older than the run.
		if o.live.pendingRedraw.Load() {
			return tickFlush
		}
		return tickStop
	}
	if !o.live.pendingRedraw.Load() && o.live.shouldSkipGlyph() {
		o.live.skippedGlyph.Add(1)
		return tickContinue
	}
	// Force redraw so time-based spinner glyphs advance.
	return tickPaint
}

// spinnerSlotSettle is how far past a glyph-slot boundary the animator
// wakes, so the paint reads the new slot despite wall-clock slew.
const spinnerSlotSettle = time.Millisecond

// untilNextSpinnerSlot is the wait from now to just past the next boundary
// of txt.SpinnerGlyph's slots. A free-running ticker has an arbitrary phase
// against those slots: two ticks could read the same glyph, the identical
// frame would be skipped, and a quiet Running row would sit stale for two
// periods. Waking once per slot makes every wake a changed frame.
func untilNextSpinnerSlot(now time.Time) time.Duration {
	period := int64(txt.SpinnerPeriod)
	return time.Duration(period-now.UnixNano()%period) + spinnerSlotSettle
}

// stampLiveFirstSeen anchors an unresolved task's heartbeat clock to the
// moment it is actually painted in the live region for the first time —
// never to declaration time, so a task declared up-front but not yet visible
// does not appear pre-aged the instant it is finally shown (evo-rec.md
// Problem 9). A no-op once set: the field only ever moves from zero once.
// liveSnapshotLocked calls it on every Task as it builds a frame.
func (t *taskState) stampLiveFirstSeen(now time.Time) {
	if t.unstampedIn(t.node.Rec.State()) {
		t.liveFirstSeenAt = now
		t.markFiling()
		t.censusStamped()
	}
}

func (o *Run) debugLiveLocked(line string) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
	}
	o.live.clear(live)
	live.WriteDurable(line)
	if o.live.visible && o.hasLiveActivityLocked() {
		o.renderLiveLocked(true)
	}
}

func (o *Run) finishLiveLocked(final string) {
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return
	}
	o.stopSpinnerAnimatorLocked()
	if o.live != nil && o.live.clear(live) {
		o.live.visible = false
	}
	// H.17 expects a compact final task line, not the full multi-section report.
	live.WriteFinal(strings.TrimRight(final, "\n"))
}

// liveSnapshotLocked is what a live frame of rows rows draws from: the
// effect sections, and every collection and standalone root Task as far
// as the frame could show it (live.LiveChildren), so building a frame
// snapshots the rows on screen rather than every Task in the run. A root
// Task already durably flushed by commitResolvedTaskLocked (a never-ran
// "fact-check" resolution — see its doc comment) is left out: it would
// otherwise reappear on the next unrelated redraw and double-print.
func (o *Run) liveSnapshotLocked(rows int, now time.Time) snapshot {
	var s snapshot
	s.Changes = o.rec.ChangeSnapshots()
	s.Plans = o.rec.PlanSnapshots()
	cols := liveCollections(o.collections, rows, now)
	s.Collections = cols.Kept()
	s = txt.WithRootCollectionTally(s, cols.Tally())
	root := live.NewLiveChildren("", rows)
	for _, t := range o.rootTasks {
		t.stampLiveFirstSeen(now)
		view := t.view()
		if t.coreEmitted || o.heldBackAsNoOpLocked(view) {
			continue
		}
		if root.Admit(&view) {
			root.Keep(t.snapshot())
		}
	}
	tasks := root.Collection(tasksSnapshot{})
	s.Tasks = tasks.Tasks
	if tally, ok := txt.ChildTallyOf(tasks); ok {
		s = txt.WithRootTally(s, tally)
	}
	return s
}

// --- folded from live_audit.go ---

// liveIndexAudit, when set, checks every live frame a collection builds
// from its childIndex against the frame a walk of every child builds. The
// tests set it, so each live test of the engine also proves the index
// current at every mutation site.
//
// rev is the record's revision before the frame was built: an audit that
// finds the record written since compares two different runs of it, and
// skips.
var liveIndexAudit func(g *tasksState, rows int, now time.Time, got tasksSnapshot, rev uint64)

// --- folded from live_census.go ---

// liveCensus counts every Task at or below one collection by what a live
// frame ranks and tallies it on. Each Task transition moves the census of
// every collection above it, so a frame can rank and tally a collection it
// cannot show without walking it: a Group of per-item Groups costs a frame
// what the screen shows, not what the run holds (E-091, E-096).
//
// slice 6: one of three counters of a collection's descendants by state, with
// the graph's tally and stateCounts; they fold into one count owned by txt.
type liveCensus struct {
	total, done, running, pending, unfinished, failed, warned int
	// unstamped counts Running or Pending Tasks no frame has painted yet
	// (taskState.stampLiveFirstSeen), so a frame skips a subtree with none.
	unstamped int
	// earliestSeen is the earliest liveFirstSeenAt below; nameWidth the
	// widest Task name, in runes.
	earliestSeen time.Time
	nameWidth    int
}

// counts is the census as the tally a live projection keeps.
func (c *liveCensus) counts() txt.ChildCounts {
	return txt.ChildCounts{
		Total: c.total, Done: c.done,
		Running: c.running > 0, Pending: c.pending > 0, Unfinished: c.unfinished > 0,
		EarliestSeen: c.earliestSeen, NameWidth: c.nameWidth,
	}
}

// rank is the attention rank of the census' most urgent Task.
func (c *liveCensus) rank() int {
	return live.AttentionRank(c.failed > 0, c.warned > 0, c.running > 0, c.pending > 0)
}

// count moves the per-state counts of one Task in state by delta.
func (c *liveCensus) count(state entityState, delta int) {
	switch state {
	case succeeded, record.Satisfied, excluded:
		c.done += delta
	case running:
		c.running += delta
	case pending:
		c.pending += delta
	case failed:
		c.failed += delta
	}
	if !record.IsTerminalTask(state) {
		c.unfinished += delta
	}
}

// seen folds a liveFirstSeenAt stamp into earliestSeen.
func (c *liveCensus) seen(at time.Time) {
	if !at.IsZero() && (c.earliestSeen.IsZero() || at.Before(c.earliestSeen)) {
		c.earliestSeen = at
	}
}

// censusesAbove calls fn on the census of every collection t sits under.
func (t *taskState) censusesAbove(fn func(*liveCensus)) {
	for c := t.collection(); c != nil; c = c.parent() {
		fn(&c.census)
	}
}

// unstamped reports whether t, in state, is a live row no frame painted.
func (t *taskState) unstampedIn(state entityState) bool {
	return (state == running || state == pending) && t.liveFirstSeenAt.IsZero()
}

// currentCensus is g's census after following the record, so it counts
// every Task by the state the record holds now.
func (g *tasksState) currentCensus() *liveCensus {
	g.out.pullRecordLocked()
	return &g.census
}

// censusDeclared counts a newly declared Task under the state it was
// declared in; censusSync moves it from there.
func (t *taskState) censusDeclared() {
	width := utf8.RuneCountInString(t.name)
	unstamped := t.unstampedIn(t.followed)
	t.censusesAbove(func(c *liveCensus) {
		c.total++
		c.count(t.followed, 1)
		if t.node.Rec.WarningCount() > 0 {
			c.warned++
		}
		if unstamped {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, width)
		c.seen(t.liveFirstSeenAt)
	})
}

// censusSync moves t from the state the census counts it under to the state
// the record holds now. Moving by the counted state, not by the transition a
// notification named, keeps the census right however late or how often the
// record tells of a change.
func (t *taskState) censusSync() {
	from, to := t.followed, t.node.Rec.State()
	if from == to {
		return
	}
	t.followed = to
	stampDelta := boolDelta(t.unstampedIn(to)) - boolDelta(t.unstampedIn(from))
	t.censusesAbove(func(c *liveCensus) {
		c.count(from, -1)
		c.count(to, 1)
		c.unstamped += stampDelta
	})
}

// censusStamped records t's first live paint.
func (t *taskState) censusStamped() {
	t.censusesAbove(func(c *liveCensus) {
		c.unstamped--
		c.seen(t.liveFirstSeenAt)
	})
}

// censusWarned records t's first warning.
func (t *taskState) censusWarned() {
	t.censusesAbove(func(c *liveCensus) { c.warned++ })
}

func boolDelta(b bool) int {
	if b {
		return 1
	}
	return 0
}

// liveOwnRow is the name g's one row renders under when it renders as its
// own Task, or "". Only a Group holding a Task of its own name can, so
// every other Group answers without being walked.
func (g *tasksState) liveOwnRow() string {
	if !g.hasNamesake {
		return ""
	}
	return live.OwnRowName(g.view())
}

// --- folded from live_meter.go ---

// renderBudget bounds the render work forced paints may spend in one
// frame interval. A forced paint renders the whole live region — every
// Task, not only the rows on screen — so forcing one per scheduler
// transition cost O(n) per Task and O(n²) per run (4000 Tasks: 46.9s).
// Under the budget every transition still paints, which is what a small
// run and FP-005's spinner-before-check need; past it, forced paints
// coalesce under the frame-rate cap and the animator paints the latest
// state on its next tick.
type renderBudget struct {
	since time.Time
	spent int
}

// forcedRenderRowsPerInterval is the budget, in Task rows rendered, forced
// paints may spend per frame interval.
const forcedRenderRowsPerInterval = 4096

// allow reports whether a forced paint of rows fits the budget at now, and
// charges it when it does.
func (b *renderBudget) allow(now time.Time, interval time.Duration, rows int) bool {
	if b.since.IsZero() || now.Sub(b.since) >= interval {
		b.since, b.spent = now, 0
	}
	if b.spent > 0 && b.spent+rows > forcedRenderRowsPerInterval {
		return false
	}
	b.spent += rows
	return true
}

// frameMeter remembers what the last live frame cost in wall time, from
// snapshot through write. Workers and the animator both report to it.
type frameMeter struct{ last atomic.Int64 }

func (m *frameMeter) observe(d time.Duration) { m.last.Store(int64(d)) }

func (m *frameMeter) cost() time.Duration { return time.Duration(m.last.Load()) }

const (
	// frameCostBudget is the most a frame may cost before the animator
	// treats the surface as overloaded: a quarter of a spinner period.
	frameCostBudget = txt.SpinnerPeriod / 4

	// liveStaleCeiling is the longest a Running row's visible frame may stay
	// unchanged (§14), as txt.SpinnerPeriod's doc states it.
	liveStaleCeiling = 100 * time.Millisecond

	// glyphSkipMargin is the slack kept under liveStaleCeiling for timer
	// and scheduling jitter.
	glyphSkipMargin = 20 * time.Millisecond
)

// skipGlyphRepaint reports whether the animator may skip this tick's
// glyph-only repaint: the last frame cost more than frameCostBudget and the
// screen was written so recently that the next tick still lands inside
// liveStaleCeiling. After a skip sinceWrite is at least one period, so it
// never skips twice in a row and cannot break the 100ms rule.
func skipGlyphRepaint(lastCost, sinceWrite time.Duration) bool {
	if lastCost <= frameCostBudget {
		return false
	}
	return sinceWrite+txt.SpinnerPeriod+spinnerSlotSettle+glyphSkipMargin < liveStaleCeiling
}

// shouldSkipGlyph applies skipGlyphRepaint to the engine's last frame and
// last write. A surface never written to is never skipped.
func (l *liveEngine) shouldSkipGlyph() bool {
	written := l.lastWrite.Load()
	if written == nil {
		return false
	}
	return skipGlyphRepaint(l.meter.cost(), wall.Since(*written))
}

// --- folded from live_paint.go ---

// liveFrame is a live region as o.mu saw it: everything render needs, owned
// by the frame. Its Snapshot is built from clones (see taskState.snapshot),
// so rendering it after o.mu is released cannot race a mutating Task.
type liveFrame struct {
	surface       liveSurface
	seq           uint64
	snap          snapshot
	width, height int
	now           time.Time
	style         txt.Style
	subject       string
	armedTitle    bool
	pane          string
}

// liveFrameLocked snapshots the live region for surface and stamps it with
// the next paint sequence number.
func (o *Run) liveFrameLocked(surface liveSurface) liveFrame {
	// Refresh geometry before layout when the driver supports it (ANSI + real TTY).
	if r, ok := surface.(interface{ RefreshSize() }); ok {
		r.RefreshSize()
	}
	cols, rows := surface.Columns(), surface.Rows()
	// Keep config width in sync for plain residual paths that still use cfg.text.
	if cols > 0 {
		o.cfg.width = cols
	}
	f := o.liveFrameAtLocked(cols, rows, o.cfg.clock.Now())
	o.live.seq++
	f.seq = o.live.seq
	f.surface = surface
	return f
}

// liveFrameAtLocked is the under-lock half of a live frame: the snapshot
// and the debug pane (§21.3.2), which is text already.
func (o *Run) liveFrameAtLocked(width, height int, now time.Time) liveFrame {
	style := o.humanStyle()
	bodyHeight := height
	var pane strings.Builder
	if recorded := o.rec.DebugRecordCount(); o.cfg.debugPresentation == debugPresentationPane && recorded > 0 {
		// Reserve rows for pane heading + visible records before budgeting the body.
		paneRows := min(debugPaneReservedRows(o.cfg.debugPane, recorded), height-1)
		bodyHeight = max(height-max(paneRows, 0), 1)
		writeDebugPane(&pane, o.rec.DebugRecordTail(o.cfg.debugPane.rows()), o.cfg.debugPane, width, style.Color)
	}
	return liveFrame{
		snap:       o.liveSnapshotLocked(bodyHeight, now),
		width:      width,
		height:     bodyHeight,
		now:        now,
		style:      style,
		subject:    o.cfg.subject,
		armedTitle: o.armedTitleLiveLocked(),
		pane:       pane.String(),
	}
}

// render is the lock-free half: the frame's text.
func (f liveFrame) render() string {
	body := live.LiveRegion(f.snap, f.height, f.width, f.now, f.style)
	if body == "" && f.armedTitle {
		body = live.ArmedTitleLine(f.subject, f.now, f.style)
	}
	if f.pane == "" {
		return live.FitLiveRegion(body, f.width)
	}
	return live.FitLiveRegion(strings.TrimRight(body+f.pane, "\n"), f.width)
}

// paint writes f unless a newer frame or a clear has superseded it, and
// reports whether it wrote. Identical bytes skip WriteLive: force bypasses
// min-gap coalescing, but a spinner tick that reads the same glyph changes
// nothing on screen.
func (l *liveEngine) paint(f liveFrame, text string) bool {
	l.paintMu.Lock()
	defer l.paintMu.Unlock()
	if f.seq <= l.floorSeq || f.seq < l.paintedSeq {
		return false
	}
	l.paintedSeq = f.seq
	if text == l.lastLiveText && l.liveActive {
		return false
	}
	start := wall.Now()
	l.lastWrite.Store(&start)
	f.surface.WriteLive(text)
	l.lastLiveText = text
	l.liveActive = true
	l.lastRender.Store(&f.now)
	l.pendingRedraw.Store(false)
	return true
}

// clear removes the live region from surface and reports whether one was on
// screen. The caller holds o.mu, so seq is stable: every frame snapshotted
// before this call is dropped, and an unlocked paint still in flight can
// never land after a ClearLive, WriteDurable or WriteFinal.
func (l *liveEngine) clear(surface liveSurface) bool {
	l.paintMu.Lock()
	defer l.paintMu.Unlock()
	l.floorSeq = l.seq
	if !l.liveActive {
		return false
	}
	surface.ClearLive()
	l.liveActive = false
	l.lastLiveText = ""
	return true
}

// paintFrameLocked snapshots, renders and writes one frame under o.mu: a
// caller that needs the frame on screen when it returns.
func (o *Run) paintFrameLocked(surface liveSurface) {
	start := wall.Now()
	f := o.liveFrameLocked(surface)
	o.live.paint(f, f.render())
	o.live.meter.observe(wall.Since(start))
}

// paintFrameUnlocked snapshots under o.mu and renders and writes with it
// released, so a slow frame never stalls the workers. Enter and leave with
// o.mu held.
func (o *Run) paintFrameUnlocked(surface liveSurface) {
	engine := o.live
	start := wall.Now()
	f := o.liveFrameLocked(surface)
	o.mu.Unlock()
	engine.paint(f, f.render())
	engine.meter.observe(wall.Since(start))
	o.mu.Lock()
}

// --- folded from phase_writer.go ---

// Writer returns a line-buffered io.Writer for narrating a talkative child
// process: each complete line (CR or LF terminated, trimmed, non-empty)
// becomes the task's live doing-text (see Doing), and every byte is also
// retained in the task's Capture ring (get-or-create, shared with
// Task.Capture) so DetailTail has proof after Fail. Lines pass through the
// same sanitize layer as Task.Doing, so hostile escape sequences never reach
// the display. Off a TTY, these mirrored lines update the live status only —
// they never force their own durable row the way an explicit
// Task.Doing call does, since the Capture ring (and its failure-path
// DetailTail) is already the child's one durable home (release-gate round 9
// finding 4). Concurrent-safe. Named Writer, not PhaseWriter (P6/rename):
// an io.Writer sink whose lines become the live-status text, following
// logrus's Logger.Writer()/zapio.Writer precedent.
//
//	cmd.Stdout = evo.Task("push").Writer()
func (t *Task) Writer() io.Writer {
	if t == nil || t.out == nil {
		return io.Discard
	}
	return &phaseWriter{task: t, evidence: t.capture()}
}

// phaseWriterMaxPendingBytes bounds the pending-line buffer: a child that
// never emits a line terminator (or emits one far longer than any phase
// text should be) would otherwise grow this buffer without limit. Once the
// pending fragment reaches this size, it is flushed as a phase line on its
// own — every byte still lands in Capture regardless, so no evidence is
// lost, only the "one line, one phase update" grouping is.
const phaseWriterMaxPendingBytes = 4 * 1024 // 4 KiB

// phaseWriter splits child output into lines (both CR and LF delimit, so a
// \r-driven progress bar updates Phase every frame) and forwards the trimmed
// text to Task.Phase, while every raw byte still feeds capture. The
// pending (not-yet-terminated) fragment is capped at
// phaseWriterMaxPendingBytes so a line-less/oversized child stream cannot
// grow it without bound.
type phaseWriter struct {
	task     *Task
	evidence *evidence

	mu  sync.Mutex
	buf []byte
}

func (w *phaseWriter) Write(p []byte) (int, error) {
	if w.evidence != nil {
		_, _ = w.evidence.Write(p)
	}
	retained := w.evidence.retainedLineCount()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		completed, width := lineEnding(w.buf[i:])
		w.buf = w.buf[i+width:]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case completed:
			w.task.appendLiveTail(trimmed, retained)
		default:
			w.task.setLiveOnlyPhase(trimmed)
		}
	}
	if leftover := strings.TrimSpace(string(w.buf)); leftover != "" {
		w.task.setLiveOnlyPhase(leftover)
		if len(w.buf) > phaseWriterMaxPendingBytes {
			w.buf = w.buf[:0]
		}
	}
	return len(p), nil
}

// lineEnding reads the delimiter at the head of rest: LF or CRLF completes a
// line; a lone CR is a progress frame redrawn in place, so it updates the
// phase without entering the live tail. width is the delimiter's byte count.
func lineEnding(rest []byte) (completed bool, width int) {
	if rest[0] == '\n' {
		return true, 1
	}
	if len(rest) > 1 && rest[1] == '\n' {
		return true, 2
	}
	return false, 1
}

// retainedLineCount is how many lines c's ring holds right now. It takes
// only c's own lock, so callers read it before taking the Output's.
func (c *evidence) retainedLineCount() int {
	root := c.root()
	if root == nil || root.ring == nil {
		return 0
	}
	return root.ring.Len()
}

// appendLiveTail records one completed Writer line: it enters the live tail
// and becomes the phase, exactly as a mirrored line always has.
func (t *Task) appendLiveTail(line string, retained int) {
	t.annotate(func(st *taskState) {
		line := txt.Text(line)
		st.tail.push(line, retained, t.out.cfg.clock.Now())
		t.out.setLiveOnlyPhaseLocked(st, line)
	})
}

// liveTailLines is how many completed Writer lines a live frame shows under
// a Running row. Fixed: the tail never grows with uptime; the Capture ring
// keeps the full record and the footer reports how many lines it holds.
const liveTailLines = 6

// liveTail is a Task's most recent completed Writer lines, oldest first, at
// most liveTailLines of them.
type liveTail struct {
	lines []string
	// retained is the Capture ring's line count as of the latest push.
	retained int
	// lastLineAt is the domain-clock time of the latest push.
	lastLineAt time.Time
}

// push appends line, evicting the oldest once full. Every completed line
// counts, even one identical to the newest: it is new output, not a redraw.
func (t *liveTail) push(line string, retained int, at time.Time) {
	t.retained = retained
	t.lastLineAt = at
	if len(t.lines) == liveTailLines {
		copy(t.lines, t.lines[1:])
		t.lines[liveTailLines-1] = line
		return
	}
	if t.lines == nil {
		t.lines = make([]string, 0, liveTailLines)
	}
	t.lines = append(t.lines, line)
}

// view is the tail as a snapshot sees it, sharing t's lines (see
// taskState.view).
func (t *liveTail) view() record.LiveTail {
	return record.LiveTail{Lines: t.lines, Evidence: t.retained, LastLineAt: t.lastLineAt}
}

var _ io.Writer = (*phaseWriter)(nil)

// --- folded from plain.go ---

// --- folded from plain_heartbeat.go ---

// plainHeartbeatInterval is how often silent long-running work earns an
// automatic durable heartbeat line in plain/non-interactive human output
// (spec §40): "• generate schema  — 30s", then "— 1m", and so on — proof
// of life for a CI log between real progress milestones. Deliberately far
// slower than the live-TTY 100ms animation contract (§23.1); the live
// renderer proves liveness with spinner motion instead.
const plainHeartbeatInterval = 30 * time.Second

// wantsPlainHeartbeatLocked reports whether Output should arm §40 heartbeats
// at all: plain/non-interactive human presentation only — the live terminal
// owns its own liveness contract — and only when the configured clock can
// schedule future callbacks (Scheduler). A TimeSource that cannot silently
// gets no heartbeat rather than a broken one.
func (o *Run) wantsPlainHeartbeatLocked() (scheduler, bool) {
	sched, ok := o.cfg.clock.(scheduler)
	if !ok {
		return nil, false
	}
	live := o.liveLocked()
	if live != nil && live.IsInteractive() && !o.cfg.plain {
		return nil, false
	}
	return sched, true
}

// armPlainHeartbeatLocked schedules st's first heartbeat check, due
// plainHeartbeatInterval after now. Called once, from promoteRunningLocked.
func (o *Run) armPlainHeartbeatLocked(st *taskState, now time.Time) {
	sched, ok := o.wantsPlainHeartbeatLocked()
	if !ok {
		return
	}
	st.heartbeat.runningAt = now
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
	o.schedulePlainHeartbeatCallbackLocked(sched, st, plainHeartbeatInterval)
}

// deferPlainHeartbeatLocked resets st's idle window from a real durable
// emission (emitTaskRunningProgressiveLocked) — "stops on any real
// milestone" (spec §40) means the currently-pending synthetic heartbeat is
// superseded, not that heartbeats are permanently revoked for this task: a
// task's very first Doing/Phase call both promotes it to Running (arming
// the heartbeat) and is itself a real emission, so a permanent one-shot
// disable would silence the heartbeat for every task that ever narrates at
// all — the common case, not the exception. Both heartbeat.runningAt (the
// elapsed display's anchor) and heartbeatDue move to now, so a task that
// goes silent again earns its next heartbeat exactly plainHeartbeatInterval
// after this real update, showing elapsed relative to that update rather
// than the task's original Running start.
func (o *Run) deferPlainHeartbeatLocked(st *taskState, now time.Time) {
	if st.heartbeat.runningAt.IsZero() {
		return
	}
	st.heartbeat.runningAt = now
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
}

// schedulePlainHeartbeatCallbackLocked arms one AfterFunc callback for id,
// due after wait. The callback re-enters checkPlainHeartbeat, which decides
// whether to emit and always re-arms the next check (self-terminating once
// the task settles — see checkPlainHeartbeat).
func (o *Run) schedulePlainHeartbeatCallbackLocked(sched scheduler, st *taskState, wait time.Duration) {
	if wait <= 0 {
		wait = plainHeartbeatInterval
	}
	id := st.id
	st.heartbeat.stop = sched.AfterFunc(wait, func() { o.checkPlainHeartbeat(id) })
}

// stopPlainHeartbeatLocked cancels st's pending heartbeat check, if any.
// Called when st settles: a settled task never earns another heartbeat.
func (o *Run) stopPlainHeartbeatLocked(st *taskState) {
	if st.heartbeat.stop == nil {
		return
	}
	st.heartbeat.stop()
	st.heartbeat.stop = nil
}

// stopPlainHeartbeatsLocked cancels every task's pending heartbeat timer.
// Close owns this so a dropped Output is unreachable even when Finish/Wait
// already settled (or never did): the AfterFunc closure otherwise keeps *Run
// alive until the 30s timer fires.
func (o *Run) stopPlainHeartbeatsLocked() {
	for _, st := range o.tasks {
		o.stopPlainHeartbeatLocked(st)
	}
}

// checkPlainHeartbeat is the deferred callback armPlainHeartbeatLocked and
// checkPlainHeartbeat itself schedule. It re-acquires the lock — the
// scheduler invokes it asynchronously, on the fake clock's Advance
// goroutine in tests or a real timer goroutine in production — and either:
//
//   - the task settled since this callback was armed: does nothing further
//     and never reschedules. settleLocked already stopped the timer; this
//     check is the backstop for a callback that was already in flight;
//   - a real durable emission pushed heartbeat.due out from under this stale
//     callback: reschedules for the corrected remaining wait without
//     emitting anything;
//   - otherwise: emits one durable heartbeat line and reschedules the next
//     interval.
func (o *Run) checkPlainHeartbeat(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || o.finished {
		// A real systemClock timer can still be in flight after Close/Finish
		// (unlike the fake clock's synchronous Advance); never write durable
		// text or reschedule past shutdown.
		return
	}
	st := o.taskStates[id]
	if st == nil || record.IsTerminalTask(st.node.Rec.State()) || st.heartbeat.runningAt.IsZero() {
		return
	}
	sched, ok := o.wantsPlainHeartbeatLocked()
	if !ok {
		return
	}
	now := o.cfg.clock.Now()
	if now.Before(st.heartbeat.due) {
		o.schedulePlainHeartbeatCallbackLocked(sched, st, st.heartbeat.due.Sub(now))
		return
	}
	o.emitPlainHeartbeatLocked(st, now)
	st.heartbeat.due = now.Add(plainHeartbeatInterval)
	o.schedulePlainHeartbeatCallbackLocked(sched, st, plainHeartbeatInterval)
}

// emitPlainHeartbeatLocked writes one durable "• <name>  — <N>s" line (spec
// §40, e.g. "• generate schema  — 30s" then "— 1m") using the same
// DisplayUnit line grammar every plain row shares, with a distinct bullet
// glyph so a heartbeat row is never mistaken for a real Running/Phase
// update. Elapsed uses the one contract §32 form (txt.FormatElapsed).
func (o *Run) emitPlainHeartbeatLocked(st *taskState, now time.Time) {
	color := !o.cfg.noColor
	elapsed := now.Sub(st.heartbeat.runningAt)
	unit := txt.DisplayUnit{
		Glyph:  txt.StyleGlyph(txt.GlyphHeartbeat.Render(o.cfg.glyphs), txt.StateColor(running), color),
		Name:   progressiveRowName(st),
		Detail: txt.Dim("— "+txt.FormatElapsed(elapsed), color),
	}
	o.writeDurableTextLocked(unit.Render("") + "\n")
}

// plainHeartbeat is one task's §40 plain-mode durable heartbeat.
type plainHeartbeat struct {
	// runningAt is the domain-clock time the task was promoted to Running
	// (see armPlainHeartbeatLocked) — the heartbeat's elapsed anchor,
	// mirroring liveFirstSeenAt's role for the live renderer's elapsed
	// suffix. Zero means no heartbeat is armed for this task (interactive
	// live presentation, or a TimeSource that cannot schedule).
	runningAt time.Time
	// due is when the next heartbeat check should actually emit a line,
	// pushed forward by any real durable emission
	// (deferPlainHeartbeatLocked) so a task that is genuinely narrating its
	// own progress never also gets a redundant heartbeat row.
	due time.Time
	// stop cancels the pending check, so a settled task's timer neither
	// fires nor keeps its Output reachable until it would have.
	stop func()
}

// --- folded from progressive.go ---

// Progressive emission implements the spirit of §1 (live becomes durable) and
// §17.5 (render immediately for terminal outcomes). Resolved items are not
// held until Finish — they stream as evidence the moment they settle.

type flusher interface {
	Flush() error
}

// emitLineProgressiveLocked streams a newly appended Line() to the human stream.
func (o *Run) emitLineProgressiveLocked() {
	pending := o.rec.LinesFrom(o.linesEmitted)
	if len(pending) == 0 {
		return
	}
	var b strings.Builder
	for _, line := range pending {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	o.linesEmitted = o.rec.LineCount()
	o.writeDurableTextLocked(b.String())
}

// writeDurableTextLocked emits durable human text immediately and flushes,
// unless a Suspend window owns the terminal: then it is held and prints in
// arrival order once the last window closes, so a sibling's row never lands
// inside a Confirm prompt.
// Interactive: above the live region on the terminal driver.
// Plain/non-interactive: primary (+ AlsoWrite) writers.
func (o *Run) writeDurableTextLocked(text string) {
	if !o.countHumanTextLocked(text) {
		return
	}
	if o.live != nil && o.live.quiesced > 0 {
		o.live.heldText = append(o.live.heldText, text)
		return
	}
	o.emitDurableTextLocked(text)
}

// writeWindowOwnerTextLocked emits durable human text the owner of an open
// Suspend window writes inside it (Confirm's prompt and its answer row): the
// window exists to show exactly this, so it is never held.
func (o *Run) writeWindowOwnerTextLocked(text string) {
	if o.countHumanTextLocked(text) {
		o.emitDurableTextLocked(text)
	}
}

// countHumanTextLocked counts text as a durable row and reports whether it
// should reach the human stream at all.
func (o *Run) countHumanTextLocked(text string) bool {
	if text == "" {
		return false
	}
	o.durableRowsEmitted++
	return !o.cfg.projection.suppressesHuman()
}

// flushHeldTextLocked prints the text held while windows were open.
func (o *Run) flushHeldTextLocked() {
	if o.live == nil {
		return
	}
	held := o.live.heldText
	o.live.heldText = nil
	for _, text := range held {
		o.emitDurableTextLocked(text)
	}
}

// emitDurableTextLocked writes text to the human stream now.
func (o *Run) emitDurableTextLocked(text string) {
	live := o.liveLocked()
	// A live region (including the armed, entity-less title line painted by
	// arm()) may still be on screen even after the surface stops reporting
	// itself interactive — terminal.ANSI disables IsInteractive() permanently
	// on a short write, but the fd keeps accepting writes and the stale frame
	// stays visible. Clear it on the surface's own bookkeeping (o.live.liveActive),
	// not on a fresh interactive re-check, so durable text is never appended
	// straight onto whatever line the live region last drew.
	if live != nil && o.live != nil {
		o.live.clear(live)
	}
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		if o.live == nil {
			o.live = &liveEngine{surface: live}
		}
		live.WriteDurable(text)
		// Redraw immediately (still holding o.mu) so the live region never sits
		// blank or stale between this durable write and whatever caller or
		// background tick next touches the terminal — the clear/write/redraw
		// cycle is one atomic sequence under one lock (evo-rec.md #12).
		if o.live.visible && o.hasLiveActivityLocked() {
			o.renderLiveLocked(true)
		}
		return
	}
	// Plain / non-interactive: stream like fmt — write now, flush now.
	writers := make([]io.Writer, 0, 1+len(o.cfg.extraWriters))
	if o.cfg.primary != nil {
		writers = append(writers, o.cfg.primary)
	}
	writers = append(writers, o.cfg.extraWriters...)
	for _, w := range writers {
		if w == nil {
			continue
		}
		_, _ = io.WriteString(w, text)
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
}

// The root name column is progressive emission's sibling-column-alignment
// width (fixture-repo-retire-dryrun.md): every root (non-collection) task
// the caller has declared so far, whether or not it has resolved yet. A
// caller that declares its whole known set of sibling tasks before
// resolving any of them (e.g. branches/worktrees/remote-tracking declared
// upfront, then worked through one at a time) gets the same aligned column
// a batch render would produce — declaring one, fully resolving it, then
// declaring the next narrows the column to whatever was known at each
// commit, which is the honest limit of "render immediately" (§17.5)
// progressive streaming: a name declared after this row already committed
// cannot retroactively widen it.
//
// rootColumn keeps that width current as root Tasks are declared (see
// appendTaskLocked), so a commit reads it without rescanning every Task.
type rootColumn struct{ count, width int }

func (c *rootColumn) add(name string) {
	c.count++
	c.width = max(c.width, utf8.RuneCountInString(name))
}

// nameWidth is the column width, or 0 when fewer than two root Tasks exist
// and there is nothing to align.
func (c rootColumn) nameWidth() int {
	if c.count < 2 {
		return 0
	}
	return c.width
}

// maxChangeSubjectWidth/maxPlanSubjectWidth are residualCompositionLocked's
// batch-time equivalent of rootColumn for the Changes/Plan ledger
// (fixture-repo-retire-dryrun.md's aligned "[planned] <name>  <verb> ..."
// column) — Changes/Plans render only at Finish (never progressively, see
// residualCompositionLocked's doc comment), so the full set is always known
// here, unlike task rows above.
// commitResolvedTaskLocked commits a resolved standalone Task's row to
// durable scrollback the instant it resolves — interactive or not — and
// drops it from the live ticker (liveSnapshotLocked already filters
// coreEmitted tasks). Collection children stay with the collection renderer
// (H.20/H.21 own their ledger via signalLiveLocked instead).
//
// Chronology contract (progressive.go's residualPlainLocked doc comment):
// progressive Task rows and Print lines interleave by resolution/call time —
// a Task that resolves before a later Println/Confirm prompt must render
// above it in scrollback. Committing at resolution time, not at Finish
// (former H.17 behavior), is what makes that true in interactive mode too;
// release-gate round 5 finding 3 is exactly a Confirm prompt or Println
// rendering above a task that had already resolved. This was previously
// Confirm-gate-only (flushGateNowLocked) because a Confirm's answer was the
// one case that couldn't wait for Finish; every standalone Task now gets the
// same immediate commit for the same reason — a later evidence call must
// never race above already-resolved work.
//
// While a Suspend window is open a sibling's row is held until it closes; the
// row of the Task that owns the window (a Confirm gate) is not, since the
// window exists to show it.
func (o *Run) commitResolvedTaskLocked(id string) {
	st := o.taskStates[id]
	if st == nil || st.coreEmitted || !record.IsTerminalTask(st.node.Rec.State()) {
		return
	}
	if o.heldBackAsNoOpLocked(st.snapshot()) {
		return
	}
	var b strings.Builder
	nameWidth := o.rootColumn.nameWidth()
	txt.WriteTaskAligned(&b, st.snapshot(), nameWidth, o.humanStyle())
	st.coreEmitted = true
	if b.Len() == 0 {
		return
	}
	if st.ownsWindow {
		o.writeWindowOwnerTextLocked(b.String())
	} else {
		o.writeDurableTextLocked(b.String())
	}
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() || o.cfg.plain {
		return
	}
	if o.hasLiveActivityLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	} else if o.live != nil && o.live.clear(live) {
		o.live.visible = false
		o.stopSpinnerAnimatorLocked()
	}
}

// hasNamedEffectRecord reports whether records holds at least one no-qty
// (evo.File/evo.Exec named) row — the "named record enumerates" half of
// "Quantity records tally; named records enumerate": Effect rows always
// carry a quantity (HasQty true) and stay Finish-only, tallied and bounded
// there exactly as before.
func hasNamedEffectRecord(records []record.EffectRecord) bool {
	for _, r := range records {
		if !r.HasQty {
			return true
		}
	}
	return false
}

// commitNamedEffectsLocked streams owner's Plan/Changes ledger section the
// instant its owning standalone task resolves (task.go's finish), provided
// the section holds at least one named (File/Exec) record — evo-rec.md's
// "a --dry user loses 'what would run' per item" fix: a caller working
// through several tasks in sequence sees each task's planned/changed items
// the moment that task's own work finishes, instead of every task's rows
// piling up at the very end of the whole run's Finish. A pure-quantity
// section (Effect) is untouched — it always
// waits for Finish, exactly as before (see hasNamedEffectRecord).
//
// This calls the same txt.WriteEffects Finish already uses (merge,
// bounded-rows cap, "+N more" overflow) so a task that records many named
// items still collapses identical (verb, object) pairs and bounds distinct
// ones — the model in o.plans/o.changes is the only place records
// accumulate; only the presentation instant moves earlier. Marking the
// section namedRowsEmitted is what makes residualCompositionLocked's Finish
// loop skip it — the raw item list must never render twice.
func (o *Run) commitNamedEffectsLocked(owner string) {
	for _, tense := range []ledgerTense{tensePlanned, tenseChanged} {
		s, ok := o.rec.Section(owner, tense)
		if !ok || s.Streamed || !hasNamedEffectRecord(s.Records) {
			continue
		}
		var b strings.Builder
		txt.WriteEffects(&b, o.effectSectionLocked(s, o.rec.MaxSubjectWidth(tense)), o.humanStyle())
		o.writeDurableTextLocked(b.String())
		o.rec.MarkSectionStreamed(owner, tense)
	}
}

// effectSectionLocked is s laid out for txt.WriteEffects — the one
// shape both the streamed and the Finish ledger txt.
func (o *Run) effectSectionLocked(s record.SectionView, nameWidth int) txt.EffectSection {
	return txt.EffectSection{
		Kind: s.Tense.String(), Subject: s.Subject, Records: s.Records,
		IntendedVerb: s.IntendedVerb, NameWidth: nameWidth, Width: o.ledgerWidthLocked(),
	}
}

// ledgerWidthLocked is the terminal width the ledger lays out for.
func (o *Run) ledgerWidthLocked() int {
	if o.cfg.width <= 0 {
		return defaultWidth
	}
	return o.cfg.width
}

// humanStyle is how this Output paints human rows.
func (o *Run) humanStyle() txt.Style {
	return txt.Style{Color: !o.cfg.noColor, Verbose: o.cfg.verbosity >= verbosityVerbose, Profile: o.cfg.glyphs}
}

// taskProgressiveTrigger names which evidence call is streaming a Running
// task's plain-mode line, so emitTaskRunningProgressiveLocked can rate-limit
// each kind independently (a phase change always streams; a progress tick
// streams once per milestone).
type taskProgressiveTrigger int

const (
	triggerPhase taskProgressiveTrigger = iota
	triggerProgress
)

// plainProgressMilestones is how many roughly-even steps a determinate
// total is divided into for plain-mode progress streaming (beginner-8): a
// durable line per increment would flood CI logs for a large total, so
// increments are thinned to this many milestones instead of streaming only
// once ("progress established") and then going silent until Done.
const plainProgressMilestones = 10

// shouldEmitPlainProgressLocked reports whether st's current progress value
// crosses a new milestone since the last plain-mode line streamed for it.
// The first tick and the final tick (completed == total) always stream —
// beginner-8's "always a final n/n" — everything between is thinned.
func shouldEmitPlainProgressLocked(st *taskState) bool {
	completed := st.node.Rec.Progress().Completed
	total := st.node.Rec.Progress().Total
	if !st.plainStream.progressStarted {
		return true
	}
	if completed == st.plainStream.progressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := max(total/plainProgressMilestones, 1)
	return completed/step != st.plainStream.progressEmitted/step
}

// progressiveRowName qualifies a streamed plain row with the subject it
// belongs to. The durable transcript indents a collection's children under
// their header, but a streamed milestone arrives on its own, far from any
// header — three sibling subjects each narrating a child called `classify`
// produce three `◐ classify  24/111` lines that name nothing. The live
// region answers the same question the same way; see
// promotesLoneChildOntoHeader.
func progressiveRowName(st *taskState) string {
	if st.collection() == nil || st.collection().name == st.name {
		return st.name
	}
	return st.collection().name + "  " + st.name
}

// emitTaskRunningProgressiveLocked streams a Running task's current
// phase/progress as a durable line in plain/non-interactive mode
// (evo-rec.md Problem 10: "Phase as static text once, then terminal rows").
// Interactive mode owns this task's presentation via the live region
// instead. A phase change streams every time its text changes; a progress
// update streams at each milestone (shouldEmitPlainProgressLocked) instead
// of flooding CI logs with every tick or going silent after the first.
//
// A child declared inside a collection streams too. Skipping it left a piped
// run with only its header line for the whole ~70s of work — the dialect's
// "never silent between the first line and Done" is not conditional on where
// the Running task happens to sit in the tree, and a collection whose
// children are explicitly named has no aggregate row streaming in its place.
func (o *Run) emitTaskRunningProgressiveLocked(st *taskState, trigger taskProgressiveTrigger) {
	if st == nil || st.node.Rec.State() != running {
		return
	}
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		return
	}
	switch trigger {
	case triggerPhase:
		if st.node.Rec.Phase() == st.plainStream.phase {
			return
		}
		st.plainStream.phase = st.node.Rec.Phase()
	case triggerProgress:
		if !shouldEmitPlainProgressLocked(st) {
			return
		}
		st.plainStream.progressStarted = true
		st.plainStream.progressEmitted = st.node.Rec.Progress().Completed
	}
	row := st.snapshot()
	row.Name = progressiveRowName(st)
	var b strings.Builder
	txt.WriteTask(&b, row, o.humanStyle())
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	// This line already proved the task is alive; the §40 heartbeat only
	// needs to cover the silence that follows real narration, not compete
	// with it (see deferPlainHeartbeatLocked).
	o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
}

// residualHasTaskRows reports whether the run declared any standalone task or
// collection at all — regardless of whether its row already streamed
// progressively or is still pending render in this call — so
// residualCompositionLocked's blank-line separator sees the run's whole
// shape, not just the slice this one call is about to write.
func residualHasTaskRows(o *Run, snap snapshot) bool {
	return len(o.tasks) > 0 || len(snap.Collections) > 0
}

// residualHasEffectSections mirrors residualHasTaskRows for the run's
// [changed]/[planned] ledger — true only when a section remains that Finish
// still owns rendering for. A named/enumerate section that already streamed
// at its task's resolution (commitNamedEffectsLocked) does not count: it
// renders nothing further here, so it must not reserve the blank-line
// separator either.
func residualHasEffectSections(o *Run) bool {
	return o.rec.HasUnstreamedSection()
}

// residualCompositionLocked is the ONE ordered sequence every human-stream
// destination reaching Finish's tail renders: unemitted lines, unresolved
// standalone tasks + collections (only for the destination that owns them —
// see includeEntities), effects (Changes/Plan — never streamed
// progressively, so every destination always renders them), the Conclusion
// band, then an optional debug-pane failure tail. Both residualPlainLocked
// (the plain/primary-mirror destination) and residualInteractiveFinalLocked
// (the live terminal's WriteFinal body) call this one function and differ
// only in includeEntities and their own write-target formatting (the
// terminal trims its trailing newline; the mirror does not) — a section
// added here reaches both destinations mechanically, closing release-gate
// round 6 finding 1 (the third parity gap between the two paths: misuse
// lines, then the debug tail, and now writeEffects — residualInteractiveFinalLocked
// used to hand-duplicate this sequence and silently drop the effects
// section, so a TTY dry-run's Plan/Changes ledger never reached the screen).
//
// includeEntities is true for the one destination that owns rendering
// unresolved tasks/collections at this Finish: the interactive live
// terminal always does (H.17 compact line); the plain/primary mirror does
// only when there is no live interactive terminal to own them instead —
// dual-stream skips them here to avoid a second render of the same rows on
// two destinations.
func (o *Run) residualCompositionLocked(snap snapshot, linesFrom int, includeEntities bool) string {
	style := o.humanStyle()
	var b strings.Builder
	for i := linesFrom; i < len(snap.Lines); i++ {
		renderplain.WriteDebugOrLine(&b, snap.Lines[i], style.Color)
	}
	if includeEntities {
		o.writeResidualEntitiesLocked(&b, snap, style)
	}
	// A blank line separates the task block from the [changed]/[planned]
	// ledger (fixture-repo-retire-dryrun.md: line 12→14) — checked against
	// the run's whole task/collection set, not just what this call happened
	// to render, since a task's row may already have streamed progressively
	// (commitResolvedTaskLocked) before Finish ever reaches this ledger.
	if residualHasTaskRows(o, snap) && residualHasEffectSections(o) {
		b.WriteByte('\n')
	}
	o.writeResidualLedgerLocked(&b, style)
	if snap.Conclusion != nil && !txt.ShouldSuppressStandaloneConclusion(snap) {
		renderplain.WriteConclusion(&b, txt.StandaloneConclusion(snap), style)
	}
	o.writeDebugTailLocked(&b, snap, style.Color)
	return b.String()
}

// writeResidualEntitiesLocked writes every root Task row not already
// streamed, then every collection, as human output shows them
// (txt.HumanProjection).
func (o *Run) writeResidualEntitiesLocked(b *strings.Builder, snap snapshot, style txt.Style) {
	nameWidth := o.rootColumn.nameWidth()
	human := txt.HumanProjection(snap, style.Verbose)
	shown := make(map[string]bool, len(human.Tasks))
	for _, t := range human.Tasks {
		shown[t.ID] = true
	}
	for _, t := range o.tasks {
		if t.collection() != nil || t.coreEmitted {
			continue
		}
		if shown[t.id] {
			txt.WriteTaskAligned(b, t.snapshot(), nameWidth, style)
		}
		t.coreEmitted = true
	}
	for _, col := range human.Collections {
		renderplain.WriteCollection(b, col, style)
	}
}

// writeResidualLedgerLocked writes every [changed] then [planned] section
// that did not already stream at its Task's resolution, sibling same-effect
// sections folded into one row (txt.FoldEffectSections).
func (o *Run) writeResidualLedgerLocked(b *strings.Builder, style txt.Style) {
	for _, tense := range []ledgerTense{tenseChanged, tensePlanned} {
		for _, sec := range o.foldedSectionsLocked(tense) {
			txt.WriteEffects(b, sec, style)
		}
	}
}

// foldSource is s as the renderer's ledger fold sees it.
func foldSource(s record.SectionView) txt.SectionSource {
	return txt.SectionSource{
		Subject: s.Subject, Records: s.Records, IntendedVerb: s.IntendedVerb,
		Containers: s.Containers, Streamed: s.Streamed,
	}
}

// foldedSectionsLocked is tense's not-yet-streamed ledger, laid out for
// txt.WriteEffects.
func (o *Run) foldedSectionsLocked(tense ledgerTense) []txt.EffectSection {
	sections := o.rec.Sections(tense)
	sources := make([]txt.SectionSource, len(sections))
	for i, s := range sections {
		sources[i] = foldSource(s)
	}
	return txt.FoldEffectSections(tense.String(), o.ledgerWidthLocked(), sources)
}

// writeDebugTailLocked writes the pane-mode diagnostic tail under the final
// result (§21.3.2). The default preserveOnBad path only fires when
// debugPaneActive is true, which only happens for a live rolling pane.
func (o *Run) writeDebugTailLocked(b *strings.Builder, snap snapshot, color bool) {
	if snap.Conclusion == nil || !o.shouldPreserveDebugTailLocked(asEvoConclusion(*snap.Conclusion)) {
		return
	}
	rows := o.cfg.debugPane.height
	if rows <= 0 {
		rows = defaultDebugPaneHeight
	}
	writeDebugTail(b, o.rec.DebugRecordTail(rows), rows, color)
}

// residualPlainLocked builds the Finish tail for the plain/primary-mirror
// human stream: only what has not already been progressive-emitted.
// renderPlain(snap, ...) still renders the full snapshot for a caller that
// wants the complete plain projection (C8: the former FinalPlain cache is
// gone — reconstruct via renderPlain(out.Snapshot(), ...)).
//
// Interactive mode: tasks/collections are owned by WriteFinal (H.17 compact
// line); this destination's own copy must not reprint them onto primary
// (same stream as Terminal) — includeEntities is false whenever a live
// interactive terminal owns them instead. See residualCompositionLocked for
// the shared ordered sequence both destinations txt.
//
// Plain order contract (P2): progressive Item/Task rows and Printf lines
// interleave by completion/call time. Residual only appends entities that
// never streamed (still-pending-until-Finish, collections, effects,
// conclusion).
func (o *Run) residualPlainLocked(snap snapshot) string {
	linesFrom := o.linesEmitted
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	text := o.residualCompositionLocked(snap, linesFrom, !interactive)
	o.linesEmitted = len(snap.Lines)
	return text
}

// residualInteractiveFinalLocked is the WriteFinal body for interactive
// mode: it always owns rendering unresolved tasks/collections (includeEntities
// true), then the same effects/Conclusion/debug-tail sections
// residualCompositionLocked shares with residualPlainLocked — one model, one
// conclusion, both surfaces. Never re-dumps already-streamed durable
// evidence (that was the double-print bug) — a never-ran standalone task
// (o.tasks, not snap.Tasks: coreEmitted lives on the internal state) already
// streamed via flushGateNowLocked/emitTaskRunningProgressiveLocked and is
// skipped by residualCompositionLocked's own coreEmitted check.
//
// linesFrom is the index into snap.Lines this call owns — captured by Finish
// before residualPlainLocked drains the same shared o.linesEmitted counter
// for its own copy, so the live terminal and any primary/AlsoWrite mirror
// each independently render the unemitted tail exactly once (release-gate
// round 5 finding 1). The trailing-newline trim is this destination's own
// write-target formatting (the terminal driver owns its own line discipline;
// the plain/primary mirror does not trim).
func (o *Run) residualInteractiveFinalLocked(snap snapshot, linesFrom int) string {
	text := o.residualCompositionLocked(snap, linesFrom, true)
	return strings.TrimRight(text, "\n")
}

// plainStreamMark is plain/non-interactive progressive streaming's
// bookkeeping for a still-Running standalone task (P10: CI logs must not
// stay silent until Finish; beginner-8: a durable line per progress
// increment, thinned to milestones for large totals).
type plainStreamMark struct {
	// phase is the last Phase text already streamed, so a repeated/no-op
	// Phase call does not re-emit.
	phase string
	// progressStarted is false until the first Progress/Bytes tick.
	progressStarted bool
	// progressEmitted is the last completed value actually streamed, so a
	// later tick knows whether it crossed a milestone boundary.
	progressEmitted int64
}

// --- folded from projection.go ---

// DataProjection selects data-command mode (UI/progress on diagnostic
// stream) — a self-documenting marker at the call site; the routing itself
// comes from pairing it with to(stderr)/withDiagnostics(stderr) (configToOptions).
func dataProjection() option {
	return optionFunc(func(*config) {})
}

// ExternalProjection selects snapshot-only host rendering.
func externalProjection() option {
	return optionFunc(func(c *config) { c.plain = true })
}

// --- folded from snapshot.go ---

func (o *Run) snapshotLocked() snapshot {
	o.followRecordLocked()
	s := snapshot{
		Version:       o.version,
		OutputID:      o.outputID,
		Subject:       o.cfg.subject,
		Lines:         o.rec.Lines(),
		Actions:       record.CloneActions(asRecordActions(o.collectActionsLocked())),
		Timestamp:     o.cfg.clock.Now(),
		DryRun:        o.cfg.dryRun,
		Preview:       o.cfg.preview,
		DryRunSubject: o.cfg.dryRunHeaderText,
		Warnings:      o.rec.RunWarnings(),
		Facts:         o.rec.RunFacts(),
	}
	for _, col := range o.collections {
		s.Collections = append(s.Collections, col.snapshot())
	}
	// Root tasks not in a collection
	for _, t := range o.tasks {
		if t.collection() == nil {
			s.Tasks = append(s.Tasks, t.snapshot())
		}
	}
	s.Changes = o.rec.ChangeSnapshots()
	s.Plans = o.rec.PlanSnapshots()
	s.Messages = o.rec.Messages()
	if c, ok := o.rec.Conclusion(); ok {
		s.Conclusion = &c
	}
	return s
}

func (t *taskState) snapshot() taskSnapshot {
	s := t.view()
	s.Problems = record.CloneProblems(s.Problems)
	s.Warnings = record.CloneProblems(s.Warnings)
	s.Facts = record.CloneFacts(s.Facts)
	s.Verification = record.CloneVerificationDetails(s.Verification)
	s.Actions = record.CloneActions(s.Actions)
	s.Skipped = cloneTaxonomy(s.Skipped)
	s.Kept = cloneTaxonomy(s.Kept)
	if t.handle != nil {
		s.Tracked = t.handle.out.taskFreshness.Tracked(t.TaskID())
	}
	tail := record.LiveTailOf(s)
	tail.Lines = slices.Clone(tail.Lines)
	return record.WithLiveTail(s, tail)
}

// view is t's snapshot sharing t's slices: read it under o.mu and never
// let it escape, since the next mutation of t can change what it shows.
// It costs no allocation, so a live frame can classify every Task by its
// view and snapshot only the ones it shows.
func (t *taskState) view() taskSnapshot {
	colID := ""
	if t.collection() != nil {
		colID = t.collection().id
	}
	truth := t.node.Rec.Truth()
	base := taskSnapshot{
		ID:           t.id,
		Key:          t.key(),
		Name:         t.name,
		State:        truth.State,
		Phase:        truth.Phase,
		ActivityAt:   truth.ActivityAt,
		Progress:     truth.Progress,
		Summary:      truth.Summary,
		Problems:     truth.Problems,
		Warnings:     truth.Warnings,
		Facts:        truth.Facts,
		Verification: truth.Verification,
		Actions:      truth.Actions,
		Skipped:      truth.Skipped,
		Kept:         truth.Kept,
		Collection:   colID,
		Declaration:  t.node.Declaration,
		DefineRan:    truth.DefineRan,
		Evidence:     truth.VerifyEvidence,

		Timing: t.timing,
	}
	return record.WithLiveTail(record.NewTaskSnapshot(base, t.liveFirstSeenAt, t.synthetic), t.tail.view())
}

func cloneTaxonomy(in []taxonomyRecord) []taxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]taxonomyRecord, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Causes) > 0 {
			out[i].Causes = append([]string(nil), out[i].Causes...)
		}
	}
	return out
}

// derivedState folds both this container's own tasks and its nested
// children (P3's recursive nesting) into one verdict: a nested Sequence or
// Group contributes exactly like one more task would, so a failure
// three levels deep still surfaces at the root header.
func (g *tasksState) derivedState() entityState {
	if g.node.BuilderFailed() {
		return failed
	}
	tasks, children := g.taskCount(), g.childStates()
	if tasks == 0 && len(children) == 0 {
		if g.node.BuilderNotStarted() {
			return notStarted
		}
		return empty
	}
	// A NotStarted child normally borrows its group's verdict from the
	// sibling that failed first, so it contributes nothing of its own. When
	// every child is NotStarted there is no such sibling — whatever stopped
	// the run was another subject entirely — and folding to Done rendered a
	// check over a subject that never ran.
	if len(children) == 0 && g.allTasksNotStarted(tasks) {
		return notStarted
	}
	v := g.settled().states.fold()
	for _, child := range children {
		switch s := child.derivedState(); s {
		case empty:
		case notStarted:
			v.unresolved = true
		default:
			v.add(s)
		}
	}
	return v.state()
}

func (g *tasksState) allTasksNotStarted(tasks int) bool {
	return tasks > 0 && g.settled().states.notStarted == tasks
}

// verdictFold accumulates member states into one container verdict. A
// Refused member counts like a Failed one does for the Tasks After the
// container (see stateOutcome): the container finished and did not
// succeed, so its header never reads Incomplete for it.
type verdictFold struct {
	running, failed, refused, cancelled, unresolved bool
}

func (v *verdictFold) add(s entityState) {
	switch s {
	case running:
		v.running = true
	case failed:
		v.failed = true
	case refused:
		v.refused = true
	case cancelled:
		v.cancelled = true
	case succeeded, record.Satisfied, excluded, notStarted:
	default:
		v.unresolved = true
	}
}

func (v verdictFold) state() entityState {
	switch {
	case v.running:
		return running
	case v.failed:
		return failed
	case v.refused:
		return refused
	case v.cancelled:
		return cancelled
	case v.unresolved:
		return incomplete
	default:
		return succeeded
	}
}

// displaySummary is g's Summary as its row shows it, given its derived
// state st: only when all children done/skipped successfully.
func (g *tasksState) displaySummary(st entityState) string {
	if st.Succeeded() && g.node.Rec.Summary() != "" && !g.hasWarnedOrFailedDescendant() {
		return g.node.Rec.Summary()
	}
	return ""
}

// hasWarnedOrFailedDescendant reports whether g or any nested child
// container carries a Failed/Cancelled task or a task with a Warn annotation
// (E2.5 finding 1): the group's own success summary must not paper over a
// warning or failure living several containers deep — the same suppression
// this method already gave Failed/Cancelled, restored and extended to
// Warnings.
func (g *tasksState) hasWarnedOrFailedDescendant() bool {
	if s := g.settled().states; s.failed > 0 || s.cancelled > 0 || s.warned > 0 {
		return true
	}
	for _, child := range g.childStates() {
		if child.hasWarnedOrFailedDescendant() {
			return true
		}
	}
	return false
}

func (o *Run) collectActionsLocked() []Action {
	seen := map[string]struct{}{}
	var out []Action
	add := func(list []Action) {
		for _, a := range list {
			k := record.ActionKey(asRecordAction(a))
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, a)
		}
	}
	for _, t := range o.tasks {
		add(asEvoActions(t.node.Rec.Actions()))
		// ZYS-848: a remedy attached via evo.Next(...) to an individual
		// Problem/warning (task.Problem(msg, evo.Next(...)),
		// task.Fail(msg, evo.Next(...))) must reach the run's own Next
		// steps the same way a task-level Next(...) call already does —
		// otherwise a remedy on one of several accumulated Problems is
		// invisible everywhere: writeProblem never renders p.Actions
		// inline (it is evidence, not a decision), and without this loop
		// it was silently dropped from the Conclusion's Next list too.
		truth := t.node.Rec.Truth()
		for _, p := range truth.Problems {
			add(asEvoActions(p.Actions))
		}
		for _, w := range truth.Warnings {
			add(asEvoActions(w.Actions))
		}
	}
	return out
}

func (g *tasksState) snapshot() tasksSnapshot {
	ts := g.header()
	for _, t := range g.taskStates() {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	for _, child := range g.childStates() {
		ts.Collections = append(ts.Collections, child.snapshot())
	}
	return ts
}

// header is g's snapshot without its children.
func (g *tasksState) header() tasksSnapshot {
	state := g.derivedState()
	return tasksSnapshot{
		ID:          g.id,
		Key:         g.key(),
		Name:        g.name,
		State:       state,
		Summary:     g.displaySummary(state),
		Declaration: g.node.Declaration,
		Sequential:  g.node.Sequential,
	}
}

// liveSnapshot is g as a live frame of rows rows can show it: the
// children it could select, snapshotted, and a tally of the rest (see
// live.LiveChildren).
func (g *tasksState) liveSnapshot(rows int, now time.Time) tasksSnapshot {
	rev := g.out.rec.Revision()
	g.stampDirectTasks(now)
	ts := g.header()
	kept, roster := g.settled().project(g, rows)
	ts = liveCollections(g.childStates(), rows, now).Into(ts)
	ts = roster.Project(ts, kept)
	if liveIndexAudit != nil {
		liveIndexAudit(g, rows, now, ts, rev)
	}
	return ts
}

// liveCollections projects cols for a live frame of rows rows: the ones
// the frame can reach through liveSnapshot, the rest tallied from views
// (see live.LiveCollections).
func liveCollections(cols []*tasksState, rows int, now time.Time) *live.LiveCollections {
	projected := live.NewLiveCollections(rows)
	for _, col := range cols {
		if census := col.currentCensus(); projected.Admit(census.rank()) {
			projected.Keep(col.liveSnapshot(rows, now))
			continue
		}
		col.stampLiveFirstSeen(now)
		projected.Omit(col.census.counts(), col.liveOwnRow())
	}
	return projected
}

// view is g's header and its Tasks' views, recursively, without the cost
// of a snapshot.
func (g *tasksState) view() tasksSnapshot {
	ts := g.header()
	for _, t := range g.taskStates() {
		ts.Tasks = append(ts.Tasks, t.view())
	}
	for _, child := range g.childStates() {
		ts.Collections = append(ts.Collections, child.view())
	}
	return ts
}

// stampLiveFirstSeen stamps every Task at or below g, as a frame that
// counted them does (see taskState.stampLiveFirstSeen).
func (g *tasksState) stampLiveFirstSeen(now time.Time) {
	if g.currentCensus().unstamped == 0 {
		return
	}
	g.stampDirectTasks(now)
	for _, child := range g.childStates() {
		child.stampLiveFirstSeen(now)
	}
}

// --- folded from structured_events.go ---

// emitWireEventLocked appends one §38 "evo.event" JSONL line for eventType,
// scoped to entityID (empty for a run-level recEvent) with payload as its
// domain fields. It is a no-op unless this Output is configured for
// FormatJSONL (spec §32.1: the event stream exists only in that Format) —
// every call site below fires unconditionally regardless of Format; this
// guard is the one place that decides whether the line is worth building.
//
// seq comes from the stream's own counter (wireSeq), not the journal's: the
// journal numbers events no line announces, which would leave gaps.
//
// A write failure is recorded rather than surfaced here (first one wins):
// this often runs deep inside the runtime's own lock, before any Task can
// meaningfully react. Finish folds wireEventErr into the misuse error it
// already returns, so "a later write fails; the Run then fails" without
// discarding the lines already written (see Finish's use of wireEventErr).
func (o *Run) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	if o.cfg.wireFormat != formatJSONL {
		return
	}
	o.wireSeq++
	ev := recEvent{
		Type:      eventType,
		Sequence:  o.wireSeq,
		Timestamp: o.cfg.clock.Now(),
		OutputID:  o.outputID,
		EntityID:  entityID,
		Payload:   payload,
	}
	if err := writeWireEventLocked(o.cfg.wireStream, ev); err != nil && o.wireEventErr == nil {
		o.wireEventErr = err
	}
}

// emitCollectionDeclaredLocked emits collection.declared (spec §38) for a
// newly declared Group/Sequence container, at top level or nested — the one
// call site Group, Sequence, and declareChildContainerLocked all share.
func (o *Run) emitCollectionDeclaredLocked(st *tasksState, parentID string) {
	kind := wire.CollectionKindGroup
	if st.node.Sequential {
		kind = wire.CollectionKindSequence
	}
	o.emitWireEventLocked(wire.EventCollectionDeclared, st.id, map[string]any{
		"name":      st.name,
		"kind":      kind,
		"parent_id": parentID,
	})
}

// --- folded from wireformat.go ---

// wireEvoVersion is the evo_version literal internal/wire's v2 encoder
// stamps onto every "evo.run" document. The engine package owns no version
// string of its own (root's PublishedRelease is the single source of
// truth, release.go) — SetWireEvoVersion lets the root package's package
// init pin it once, so there is exactly one place a release bump has to
// touch.
var wireEvoVersion = "dev"

// SetWireEvoVersion pins the evo_version literal FormatJSON/FormatJSONL
// stamp onto the v2 wire documents they emit. Called once, by the root
// package's init(), from evo.PublishedRelease.
func setWireEvoVersion(v string) { wireEvoVersion = v }

// writeWireRunLocked encodes conc as the final "evo.run" document and
// writes it plus a trailing newline to w (spec §53: "plus one trailing
// newline"). A nil w or nil conc is a no-op. Encode/write failures are
// wrapped in ErrRenderer, matching writeMachinePresentation's legacy-wire
// contract (spec §32.2: "a genuine Run failure, not an ignored logging
// error").
func writeWireRunLocked(w io.Writer, conc Conclusion) error {
	if w == nil {
		return nil
	}
	body, err := wire.EncodeRun(record.Result{Conclusion: asRecordConclusion(conc)}, wireEvoVersion)
	if err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	body = append(body, '\n')
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}

// writeWireEventLocked encodes e as one "evo.event" JSONL line and writes
// it to w — the FormatJSONL counterpart of writeStreamJSONLocked's legacy
// "0.3" streamed-JSON line, called from emitWireEventLocked
// (structured_events.go). A write failure here does not stop the stream —
// earlier lines already written stay valid (spec §32.2: "Earlier valid
// lines remain valid if a later write fails") — but it is returned so the
// caller can latch it as this Run's first wire-write failure and fail the
// Run at Finish (spec §32.2: "... the Run then fails"), instead of the
// error silently vanishing.
func writeWireEventLocked(w io.Writer, e recEvent) error {
	if w == nil {
		return nil
	}
	row, err := wire.EncodeEvent(e)
	if err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	row = append(row, '\n')
	if _, err := w.Write(row); err != nil {
		return fmt.Errorf("%w: %v", errRenderer, err)
	}
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
	return nil
}

// Output selects the presentation encoding (CONTRACT §6).
type Output int

const (
	Human Output = iota + 1
	Plain
	JSON
	JSONL
)

// runStartedPayload carries the run's title on run.started; an untitled run
// has no payload.
func runStartedPayload(title string) map[string]any {
	if title == "" {
		return nil
	}
	return map[string]any{"name": title}
}
