//go:build !unix

package terminal

// resizeWatch is unused on non-unix builds (field present for struct layout).
type resizeWatch struct{}

// StartResizeWatch is a no-op on platforms without SIGWINCH (e.g. Windows).
// Geometry still updates via RefreshSize on each live redraw when WithSizeFile
// is set (query term size when available).
func (a *ANSI) StartResizeWatch(onResize func()) {}

// StopResizeWatch is a no-op on non-unix builds.
func (a *ANSI) StopResizeWatch() {}
