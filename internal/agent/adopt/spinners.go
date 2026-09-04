package adopt

// spinnerImports are known manual-spinner/progress-bar libraries evo's
// built-in Task heartbeat replaces outright (docs/guides/teaching-ladder.md).
var spinnerImports = map[string]string{
	"github.com/briandowns/spinner":     "evo.Task's live heartbeat renders progress/elapsed automatically — delete the manual spinner and its Start/Stop calls.",
	"github.com/schollz/progressbar":    "evo.Task(name).Each(items) (or Progress) owns absolute progress rendering — delete the manual progress bar.",
	"github.com/schollz/progressbar/v3": "evo.Task(name).Each(items) (or Progress) owns absolute progress rendering — delete the manual progress bar.",
	"github.com/vbauerster/mpb":         "evo.Task(name).Each(items) owns concurrent progress rendering — delete the manual multi-bar container.",
	"github.com/vbauerster/mpb/v8":      "evo.Task(name).Each(items) owns concurrent progress rendering — delete the manual multi-bar container.",
	"github.com/cheggaaa/pb":            "evo.Task(name).Each(items) (or Progress) owns absolute progress rendering — delete the manual progress bar.",
	"gopkg.in/cheggaaa/pb.v1":           "evo.Task(name).Each(items) (or Progress) owns absolute progress rendering — delete the manual progress bar.",
	"github.com/pterm/pterm":            "evo.Task/Sequence/DisplayGroup own spinners, progress, and hierarchical output — inventory which pterm widgets are in play before replacing.",
}
