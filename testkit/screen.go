package testkit

// Screen is a minimal virtual terminal stub for v0.1–v0.2 tests.
// Full VT semantics (H.17/H.20–H.22) land with interactive projection.
type Screen struct {
	width, height int
	interactive   bool
	noColor       bool
	liveFrames    int
	finalText     string
	ops           []Operation
}

// Operation is a recorded terminal operation (v0.2 interactive).
type Operation struct {
	Kind string
	Text string
}

// ScreenOption configures a Screen.
type ScreenOption func(*Screen)

// Interactive marks the screen as a TTY-like device.
func Interactive() ScreenOption {
	return func(s *Screen) { s.interactive = true }
}

// Width sets columns.
func Width(n int) ScreenOption {
	return func(s *Screen) { s.width = n }
}

// Height sets rows.
func Height(n int) ScreenOption {
	return func(s *Screen) { s.height = n }
}

// NoColor disables color.
func NoColor() ScreenOption {
	return func(s *Screen) { s.noColor = true }
}

// NewScreen builds a virtual terminal screen.
func NewScreen(opts ...ScreenOption) *Screen {
	s := &Screen{width: 80, height: 24}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ID implements evo.TerminalDriver.
func (s *Screen) ID() string { return "testkit-screen" }

// LiveFrameCount returns recorded live frames (0 until interactive renderer).
func (s *Screen) LiveFrameCount() int { return s.liveFrames }

// FinalText returns durable final text.
func (s *Screen) FinalText() string { return s.finalText }

// Operations returns recorded ops.
func (s *Screen) Operations() []Operation { return append([]Operation(nil), s.ops...) }

// LatestLiveText returns the last live frame text.
func (s *Screen) LatestLiveText() string {
	for i := len(s.ops) - 1; i >= 0; i-- {
		if s.ops[i].Kind == "live" {
			return s.ops[i].Text
		}
	}
	return ""
}

// DrawLive records a live frame (used by future terminal driver tests).
func DrawLive(text string) Operation { return Operation{Kind: "live", Text: text} }

// ClearLive records a live-region clear.
func ClearLive() Operation { return Operation{Kind: "clear"} }

// WriteDurable records a durable line insert.
func WriteDurable(text string) Operation { return Operation{Kind: "durable", Text: text} }

// WriteFinal records final output.
func WriteFinal(text string) Operation { return Operation{Kind: "final", Text: text} }

// DiffOperations compares expected vs got operation sequences.
func DiffOperations(want, got []Operation) string {
	if len(want) != len(got) {
		return "length mismatch"
	}
	for i := range want {
		if want[i] != got[i] {
			return "operation mismatch at " + want[i].Kind
		}
	}
	return ""
}
