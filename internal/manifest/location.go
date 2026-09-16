package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime/debug"
)

// manifestFileName is the on-disk manifest file's fixed basename (spec
// §11.3: ".../manifest-v1.json").
const manifestFileName = "manifest-v1.json"

// Environment is the facade Locate reads the executable path, its build
// info, and the user cache directory through, instead of os/runtime
// directly (facade rule).
type Environment interface {
	Executable() (string, error)
	UserCacheDir() (string, error)
	// ReadBuildInfo returns the running binary's main module path, or ok=false
	// when no build info is available (e.g. built without modules).
	ReadBuildInfo() (mainModulePath string, ok bool)
}

// Config selects where a Store's manifest file lives and which
// application/workspace identity it is scoped to.
type Config struct {
	// AppID overrides the derived application id outright.
	AppID string
	// StateDir, when non-empty, is the exact state directory: the manifest
	// path becomes <StateDir>/manifest-v1.json and default derivation
	// (cache dir/app-id/workspace-hash) is bypassed entirely.
	StateDir string
	// Workspace is the canonical working directory captured once at Run
	// start (spec §11.3) — hashed into the default derived path so distinct
	// workspaces never share one manifest.
	Workspace string
}

// Locate resolves Config into the manifest file path this Store will read
// and write, using env for the executable/build-info/cache-dir facades.
func Locate(cfg Config, env Environment) (string, error) {
	if cfg.StateDir != "" {
		return filepath.Join(cfg.StateDir, manifestFileName), nil
	}
	appID := cfg.AppID
	if appID == "" {
		appID = deriveAppID(env)
	}
	cacheDir, err := env.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("manifest: resolve user cache dir: %w", err)
	}
	workspaceHash := hex.EncodeToString(sha256Sum(cfg.Workspace))
	return filepath.Join(cacheDir, "evident-output", appID, workspaceHash, manifestFileName), nil
}

// deriveAppID is the Go main-module path plus the executable's basename
// when build info is available, otherwise the executable's basename alone
// (spec §11.3).
func deriveAppID(env Environment) string {
	exe, err := env.Executable()
	base := "app"
	if err == nil && exe != "" {
		base = filepath.Base(exe)
	}
	if modulePath, ok := env.ReadBuildInfo(); ok && modulePath != "" {
		return modulePath + "/" + base
	}
	return base
}

func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// osEnvironment is the real process/filesystem Environment.
type osEnvironment struct{}

// NewOSEnvironment returns the production Environment facade.
func NewOSEnvironment() Environment { return osEnvironment{} }

func (osEnvironment) Executable() (string, error) { return realExecutable() }

func (osEnvironment) ReadBuildInfo() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path == "" {
		return "", false
	}
	return info.Main.Path, true
}

func (osEnvironment) UserCacheDir() (string, error) { return realUserCacheDir() }
