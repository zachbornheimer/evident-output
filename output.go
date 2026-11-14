package evo

import (
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// Output selects the presentation encoding (CONTRACT §6).
type Output int

const (
	Human Output = iota + 1
	Plain
	JSON
	JSONL
)

// Environment keys honored at run start when the matching Config field is unset.
const (
	envKeyOutput     = "EVO_OUTPUT"
	envKeyVerbose    = "EVO_VERBOSE"
	envKeyDebug      = "EVO_DEBUG"
	envKeyNoColor    = "NO_COLOR"
	envKeyForceColor = "FORCE_COLOR"
	envKeyLocaleAll  = "LC_ALL"
	envKeyLocaleType = "LC_CTYPE"
	envKeyLang       = "LANG"
)

const (
	envForceColorOff = "0"
	envVerboseOn     = "1"
)

// envOutputs are EVO_OUTPUT's values; any other value leaves Output to terminal inference.
var envOutputs = map[string]Output{"human": Human, "plain": Plain, "json": JSON, "jsonl": JSONL}

// envDebugLevels are EVO_DEBUG's values.
var envDebugLevels = map[string]logLevel{"info": levelInfo, "debug": levelDebug, "warn": levelWarn, "error": levelError}

var (
	lookupEnvMu sync.RWMutex
	lookupEnvFn = process.Env
)

func lookupEnv(key string) string {
	lookupEnvMu.RLock()
	fn := lookupEnvFn
	lookupEnvMu.RUnlock()
	if fn == nil {
		return process.Env(key)
	}
	return fn(key)
}

// outputFromEnv is the Output EVO_OUTPUT's value v names, zero when it names none.
func outputFromEnv(v string) Output { return envOutputs[strings.TrimSpace(v)] }

// forceColorRequested reports whether FORCE_COLOR asks for color; NO_COLOR wins, so it is checked first.
func forceColorRequested() bool {
	if lookupEnv(envKeyNoColor) != "" {
		return false
	}
	force := strings.TrimSpace(lookupEnv(envKeyForceColor))
	return force != "" && force != envForceColorOff
}

// parseDebugEnv is the journal level EVO_DEBUG's value v names.
func parseDebugEnv(v string) (logLevel, bool) {
	lvl, ok := envDebugLevels[strings.TrimSpace(v)]
	return lvl, ok
}
