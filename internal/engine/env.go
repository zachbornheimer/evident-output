package engine

import (
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// Environment keys honored at Init when the matching Config field is unset.
const (
	envKeyOutput  = "EVO_OUTPUT"
	envKeyColor   = "EVO_COLOR"
	envKeyVerbose = "EVO_VERBOSE"
	envKeyDebug   = "EVO_DEBUG"
	envKeyNoColor = "NO_COLOR"
)

const (
	envOutputHuman         = "human"
	envOutputPlain         = "plain"
	envOutputJSON          = "json"
	envOutputJSONL         = "jsonl"
	envOutputStreamJSON    = "stream-json"
	envOutputStreamJSONAlt = "stream_json"

	envColorAuto   = "auto"
	envColorAlways = "always"
	envColorNever  = "never"

	envVerboseOn = "1"

	envDebugInfo  = "info"
	envDebugDebug = "debug"
	envDebugTrace = "trace"
)

// Projection selects presentation encoding. Independent of Format, which
// only routes streams (stdout vs stderr). The zero value is unspecified:
// Init fills it from EVO_OUTPUT when set, otherwise human TTY/plain inference.
type Projection int

const (
	// ProjectionHuman is ordinary TTY/plain inference. Non-zero so an
	// explicit Config{Projection: ProjectionHuman} wins over EVO_OUTPUT.
	ProjectionHuman Projection = iota + 1
	// ProjectionPlain forces the durable report (no live region).
	ProjectionPlain
	// ProjectionJSON writes a JSONDocument at Finish.
	ProjectionJSON
	// ProjectionJSONL writes Event JSON Lines at Finish.
	ProjectionJSONL
	// ProjectionStreamJSON writes one EventJSON line at each journal append.
	ProjectionStreamJSON
)

func (p Projection) forcesPlain() bool {
	switch p {
	case ProjectionPlain, ProjectionJSON, ProjectionJSONL, ProjectionStreamJSON:
		return true
	default:
		return false
	}
}

func (p Projection) suppressesHuman() bool {
	switch p {
	case ProjectionJSON, ProjectionJSONL, ProjectionStreamJSON:
		return true
	default:
		return false
	}
}

var (
	lookupEnvMu sync.RWMutex
	lookupEnvFn = process.Env
)

// SwapLookupEnv replaces the process-environment facade. Tests inject a
// map instead of racing os.Setenv. Restore via the returned function.
func SwapLookupEnv(fn func(string) string) func() {
	lookupEnvMu.Lock()
	prev := lookupEnvFn
	lookupEnvFn = fn
	lookupEnvMu.Unlock()
	return func() {
		lookupEnvMu.Lock()
		lookupEnvFn = prev
		lookupEnvMu.Unlock()
	}
}

// lookupEnv is the process-environment facade (same idea as processArgv0).
func lookupEnv(key string) string {
	lookupEnvMu.RLock()
	fn := lookupEnvFn
	lookupEnvMu.RUnlock()
	if fn == nil {
		return process.Env(key)
	}
	return fn(key)
}

// applyEnv fills zero Config fields from the process environment. Explicit
// Config values already set are left alone. Options-path Init skips this.
func applyEnv(c Config) Config {
	if c.Projection == 0 {
		if p, ok := parseOutputEnv(lookupEnv(envKeyOutput)); ok {
			c = selectOutputEnv(c, p)
		}
	}
	if c.Projection.forcesPlain() {
		c.Plain = true
	}
	if c.Color == ColorAuto {
		if mode, ok := parseColorEnv(lookupEnv(envKeyColor)); ok {
			c.Color = mode
		}
	}
	if c.Verbosity == VerbosityNormal && lookupEnv(envKeyVerbose) == envVerboseOn {
		c.Verbosity = VerbosityVerbose
	}
	if c.Debug.Level == LevelUnset {
		if lvl, ok := parseDebugEnv(lookupEnv(envKeyDebug)); ok {
			c.Debug.Level = lvl
		}
	}
	return c
}

// selectOutputEnv applies EVO_OUTPUT's projection p. json and jsonl
// select the evo.run document and evo.event stream (FormatJSON,
// FormatJSONL) when no Format was chosen: those carry every structured
// Fact, disposition and Problem, where the frozen output.v1 projection
// cannot (E-090). A Format the caller chose keeps stdout for itself, so
// there p stays the output.v1 projection on stderr.
func selectOutputEnv(c Config, p Projection) Config {
	switch {
	case c.Format == FormatHuman && p == ProjectionJSON:
		c.Format = FormatJSON
	case c.Format == FormatHuman && p == ProjectionJSONL:
		c.Format = FormatJSONL
	default:
		c.Projection = p
	}
	return c
}

func parseOutputEnv(v string) (Projection, bool) {
	switch strings.TrimSpace(v) {
	case envOutputHuman:
		return ProjectionHuman, true
	case envOutputPlain:
		return ProjectionPlain, true
	case envOutputJSON:
		return ProjectionJSON, true
	case envOutputJSONL:
		return ProjectionJSONL, true
	case envOutputStreamJSON, envOutputStreamJSONAlt:
		return ProjectionStreamJSON, true
	default:
		return 0, false
	}
}

func parseColorEnv(v string) (ColorMode, bool) {
	switch strings.TrimSpace(v) {
	case envColorAuto:
		return ColorAuto, true
	case envColorAlways:
		return ColorAlways, true
	case envColorNever:
		return ColorNever, true
	default:
		return 0, false
	}
}

func parseDebugEnv(v string) (LogLevel, bool) {
	switch strings.TrimSpace(v) {
	case envDebugInfo:
		return LevelInfo, true
	case envDebugDebug:
		return LevelDebug, true
	case envDebugTrace:
		return LevelTrace, true
	default:
		return 0, false
	}
}
