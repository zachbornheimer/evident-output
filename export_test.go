package evo

import "io"

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

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func MarkWriterAsCharDevice(w io.Writer) func() {
	charDeviceOverride.mu.Lock()
	prevW, prevOn := charDeviceOverride.writer, charDeviceOverride.on
	charDeviceOverride.writer = w
	charDeviceOverride.on = true
	charDeviceOverride.mu.Unlock()
	return func() {
		charDeviceOverride.mu.Lock()
		charDeviceOverride.writer = prevW
		charDeviceOverride.on = prevOn
		charDeviceOverride.mu.Unlock()
	}
}
