package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
)

// ErrAppFingerprintUnavailable is returned by App's Fingerprint when neither
// the running executable's bytes nor a Go build ID could be read — spec
// §11.2's "unavailable" outcome.
var ErrAppFingerprintUnavailable = errors.New("fingerprint: application fingerprint unavailable")

// appEnvironment is the facade App() reads the running executable and its
// build metadata through — swapped for a fake in this package's own tests.
type appEnvironment interface {
	Executable() (string, error)
	ReadFile(path string) ([]byte, error)
	ReadBuildInfo() (buildID string, ok bool)
}

// osAppEnvironment is the real process/filesystem.
type osAppEnvironment struct{}

func (osAppEnvironment) Executable() (string, error)          { return os.Executable() }
func (osAppEnvironment) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osAppEnvironment) ReadBuildInfo() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return info.Main.Path + "@" + setting.Value, true
		}
	}
	if info.Main.Sum != "" {
		return info.Main.Path + "@" + info.Main.Sum, true
	}
	return "", false
}

// activeAppEnvironment is the package-level facade seam (see fs.go's
// activeFS for the same pattern applied to filesystem observation).
var activeAppEnvironment appEnvironment = osAppEnvironment{}

// withAppEnvironment runs fn with the app facade swapped to env, restoring
// the previous one afterward. Test-only.
func withAppEnvironment(env appEnvironment, fn func()) {
	prev := activeAppEnvironment
	activeAppEnvironment = env
	defer func() { activeAppEnvironment = prev }()
	fn()
}

// appFingerprint implements Fingerprint for App().
type appFingerprint struct{}

// App fingerprints the running application itself (spec §11.2): SHA-256 of
// the running executable's bytes when readable, else a stable Go build ID
// when build information is available, else ErrAppFingerprintUnavailable.
// Include it in an operation's Basis only when the application's own
// implementation is itself a semantic input to that operation's result.
func App() Fingerprint {
	return appFingerprint{}
}

func (appFingerprint) Fingerprint(_ context.Context) (FingerprintValue, error) {
	digest, err := appDigest(activeAppEnvironment)
	if err != nil {
		return FingerprintValue{}, err
	}
	return FingerprintValue{Kind: KindApp, Key: "application", Digest: digest}, nil
}

func appDigest(env appEnvironment) (Digest, error) {
	if exe, err := env.Executable(); err == nil {
		if bytes, err := env.ReadFile(exe); err == nil {
			return sum256(append([]byte(markerRegularFile), bytes...)), nil
		}
	}
	if buildID, ok := env.ReadBuildInfo(); ok {
		return sum256(append([]byte("evident-output:fingerprint:app:buildid:v1\x00"), buildID...)), nil
	}
	return Digest{}, fmt.Errorf("app: %w", ErrAppFingerprintUnavailable)
}
