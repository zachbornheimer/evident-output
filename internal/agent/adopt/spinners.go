package adopt

// spinnerImports are known manual-spinner/progress-bar libraries evo's
// built-in Task heartbeat replaces outright (docs/guides/teaching-ladder.md).
var spinnerImports = map[string]string{
	"github.com/briandowns/spinner":     "evo.Task's live heartbeat renders progress/elapsed automatically — delete the manual spinner and its Start/Stop calls.",
	"github.com/schollz/progressbar":    "evo.Group(name).Each(items) owns collection progress — delete the manual progress bar.",
	"github.com/schollz/progressbar/v3": "evo.Group(name).Each(items) owns collection progress — delete the manual progress bar.",
	"github.com/vbauerster/mpb":         "evo.Group(name).Each(items) owns concurrent collection progress — delete the manual multi-bar container.",
	"github.com/vbauerster/mpb/v8":      "evo.Group(name).Each(items) owns concurrent collection progress — delete the manual multi-bar container.",
	"github.com/cheggaaa/pb":            "evo.Group(name).Each(items) owns collection progress — delete the manual progress bar.",
	"gopkg.in/cheggaaa/pb.v1":           "evo.Group(name).Each(items) owns collection progress — delete the manual progress bar.",
	"github.com/pterm/pterm":            "evo.Task/Sequence/Group own spinners, progress, and hierarchical output — inventory which pterm widgets are in play before replacing.",
}
