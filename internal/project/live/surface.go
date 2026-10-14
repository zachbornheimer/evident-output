package live

import "github.com/zachbornheimer/evident-output/v2/internal/terminal"

// Surface is an interactive terminal a live region paints on. testkit.Screen
// and terminal.ANSI implement it.
type Surface interface {
	terminal.Driver
	Columns() int
	Rows() int
	IsInteractive() bool
	WriteLive(text string)
	ClearLive()
	WriteDurable(line string)
	WriteFinal(text string)
}

// SurfaceOf is d as a Surface, or nil when d cannot paint a live region.
func SurfaceOf(d terminal.Driver) Surface {
	if s, ok := d.(Surface); ok {
		return s
	}
	return nil
}

// resizeWatcher is implemented by terminal.ANSI (SIGWINCH); other surfaces
// never resize.
type resizeWatcher interface {
	StartResizeWatch(onResize func())
	StopResizeWatch()
}

// sizeRefresher is a surface that re-reads its geometry on request.
type sizeRefresher interface{ RefreshSize() }
