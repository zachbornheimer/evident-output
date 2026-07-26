package testkit

// Screen is a virtual terminal that records live-region operations for tests.
type Screen struct {
	width, height int
	interactive   bool
	noColor       bool
	liveFrames    int
	finalText     string
	ops           []Operation
	latestLive    string
}

// Operation is a recorded terminal operation.
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
	s := &Screen{width: 80, height: 24, interactive: false}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ID implements evo.TerminalDriver.
func (s *Screen) ID() string { return "testkit-screen" }

// Columns implements evo.LiveSurface.
func (s *Screen) Columns() int {
	if s.width <= 0 {
		return 80
	}
	return s.width
}

// Rows implements evo.LiveSurface.
func (s *Screen) Rows() int {
	if s.height <= 0 {
		return 24
	}
	return s.height
}

// IsInteractive implements evo.LiveSurface.
func (s *Screen) IsInteractive() bool { return s.interactive }

// WriteLive implements evo.LiveSurface.
func (s *Screen) WriteLive(text string) {
	s.liveFrames++
	s.latestLive = text
	s.ops = append(s.ops, Operation{Kind: "live", Text: text})
}

// ClearLive implements evo.LiveSurface.
func (s *Screen) ClearLive() {
	s.ops = append(s.ops, Operation{Kind: "clear"})
}

// WriteDurable implements evo.LiveSurface.
func (s *Screen) WriteDurable(line string) {
	s.ops = append(s.ops, Operation{Kind: "durable", Text: line})
}

// WriteFinal implements evo.LiveSurface.
func (s *Screen) WriteFinal(text string) {
	s.finalText = text
	s.ops = append(s.ops, Operation{Kind: "final", Text: text})
}

// LiveFrameCount returns recorded live frames.
func (s *Screen) LiveFrameCount() int { return s.liveFrames }

// FinalText returns durable final text.
func (s *Screen) FinalText() string { return s.finalText }

// Operations returns recorded ops.
func (s *Screen) Operations() []Operation { return append([]Operation(nil), s.ops...) }

// LatestLiveText returns the last live frame text.
func (s *Screen) LatestLiveText() string { return s.latestLive }

// SetSize updates columns/rows mid-session (CON-004 resize storm).
func (s *Screen) SetSize(width, height int) {
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
	s.ops = append(s.ops, Operation{Kind: "resize", Text: ""})
}

// DrawLive records a live frame expectation helper.
func DrawLive(text string) Operation { return Operation{Kind: "live", Text: text} }

// ClearLiveOp is an expectation helper (ClearLive method exists on Screen).
func ClearLive() Operation { return Operation{Kind: "clear"} }

// WriteDurableOp expectation helper name collision — use WriteDurable as func.
func WriteDurable(text string) Operation { return Operation{Kind: "durable", Text: text} }

// WriteFinal expectation helper.
func WriteFinal(text string) Operation { return Operation{Kind: "final", Text: text} }

// DiffOperations compares expected vs got operation sequences.
func DiffOperations(want, got []Operation) string {
	if len(want) != len(got) {
		return "length mismatch"
	}
	for i := range want {
		if want[i].Kind != got[i].Kind || want[i].Text != got[i].Text {
			return "operation mismatch at index " + itoa(i) +
				" want=" + want[i].Kind + ":" + want[i].Text +
				" got=" + got[i].Kind + ":" + got[i].Text
		}
	}
	return ""
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
