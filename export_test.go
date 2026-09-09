package evo

import (
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// TestClock is the deterministic TimeSource for evo_test.
type TestClock = FixedClock

// TestSystemClock is the wall-clock TimeSource for evo_test.
type TestSystemClock = SystemClock

// TestRedactor is the identity Redactor for evo_test.
type TestRedactor = NoopRedactor

type TestScope = Scope
type TestEvidence = Evidence

func DelayForTest(d time.Duration) *time.Duration { return delay(d) }

func AlsoWrite(w io.Writer) Option               { return alsoWrite(w) }
func Clock(ts TimeSource) Option                 { return clock(ts) }
func DataProjection() Option                     { return dataProjection() }
func DebugAddSource() Option                     { return debugAddSource() }
func DebugHistory() Option                       { return debugHistory() }
func DebugLevel(level LogLevel) Option           { return debugLevel(level) }
func DebugPane(opts ...DebugPaneOption) Option   { return debugPane(opts...) }
func Diagnostics(w io.Writer) Option             { return diagnostics(w) }
func DryRun() Option                             { return dryRun() }
func ExternalProjection() Option                 { return externalProjection() }
func MaxEntities(n int) Option                   { return maxEntities(n) }
func MaxEvents(n int) Option                     { return maxEvents(n) }
func MaxFrameRate(framesPerSecond int) Option    { return maxFrameRate(framesPerSecond) }
func NoColor() Option                            { return noColor() }
func Plain() Option                              { return plain() }
func Redact(r Redactor) Option                   { return redact(r) }
func ResultStream(w io.Writer) Option            { return resultStream(w) }
func Stdin(r io.Reader) Option                   { return stdin(r) }
func Strict() Option                             { return strict() }
func Terminal(driver TerminalDriver) Option      { return terminal(driver) }
func Title(subject string) Option                { return title(subject) }
func To(w io.Writer) Option                      { return to(w) }
func VisibilityDelay(delay time.Duration) Option { return visibilityDelay(delay) }
func Width(columns int) Option                   { return width(columns) }

func Delay(d time.Duration) *time.Duration { return delay(d) }
func IsCharDevice(w io.Writer) bool        { return isCharDevice(w) }
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}
func EncodeJSON(s Snapshot) ([]byte, error)         { return encodeJSON(s) }
func EncodeJSONL(events []Event) ([]byte, error)    { return encodeJSONL(events) }
func EncodeEventJSON(e Event) ([]byte, error)       { return encodeEventJSON(e) }
func SlogHandler() slog.Handler                     { return slogHandler() }
func MainWith(out *Output, run func(*Output) error) { mainWith(out, run) }
func KeepLastLines(n int) EvidenceOption            { return keepLastLines(n) }
func MaxEvidenceBytes(n int) EvidenceOption         { return maxEvidenceBytes(n) }
func MirrorToDebug() EvidenceOption                 { return mirrorToDebug() }
func MirrorToDiagnostics() EvidenceOption           { return mirrorToDiagnostics() }
func NewestFirst() DebugPaneOption                  { return newestFirst() }
func OldestFirst() DebugPaneOption                  { return oldestFirst() }
func PaneHeight(lines int) DebugPaneOption          { return paneHeight(lines) }
func PreserveDebugTail() DebugPaneOption            { return preserveDebugTail() }
func Affected(n int) EffectOption                   { return affected(n) }
func Glyphs(p GlyphProfile) Option                  { return glyphs(p) }
func ID(id string) EntityOption                     { return engine.ID(id) }
func StartPhase(text string) EntityOption           { return engine.StartPhase(text) }

func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return engine.ReasonConstrainedForTest(name, opts...)
}

func SwapLookupEnv(fn func(string) string) func() {
	return engine.SwapLookupEnv(fn)
}

func MarkWriterAsCharDevice(w io.Writer) func() {
	return engine.MarkWriterAsCharDevice(w)
}

func SlogHandlerForTest() slog.Handler { return slogHandler() }
