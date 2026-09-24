package review

import "strings"

// firstPaintIOMarkers are calls heavy enough to blank the terminal for a
// visible interval when run ahead of the first paint. Domain inventory
// (purge.Inventory) is the canary that stdlib-only markers missed.
var firstPaintIOMarkers = []string{
	"os.ReadFile(", "os.ReadDir(", "os.Open(", "filepath.Walk(",
	"filepath.WalkDir(", "exec.Command(", "http.Get(", "net.Dial(",
	".Inventory(",
}

// firstPaintInitMarkers arm the display (evo-rec.md "First paint").
var firstPaintInitMarkers = []string{"evo.Init("}

// firstPaintFallbackInitMarkers arm the display when no Init call exists.
var firstPaintFallbackInitMarkers = []string{"evo.New("}

// firstPaintEntityMarkers declare the first presentation entity.
var firstPaintEntityMarkers = []string{".Task(", ".Group(", ".Sequence(", ".Item("}

// loopWalkMarkers are domain walk helpers that count as loop work even
// without a stdlib I/O spelling.
var loopWalkMarkers = []string{"FindWorktree", "FindCache", "ListWorktree", "WalkDir", "Walk("}

// paintWindow is the span of a function body that is armed but still blank:
// from the evo.Init/New call to the first entity declared after it (or the
// end of the body when none follows). Entities declared before the init call
// — closures defined ahead of it — never close the window.
type paintWindow struct {
	start, end int
}

// armedWindow locates body's paint window; ok is false when body never arms
// the display.
func armedWindow(body string) (w paintWindow, ok bool) {
	start := earliestIndex(body, firstPaintInitMarkers)
	if start < 0 {
		start = earliestIndex(body, firstPaintFallbackInitMarkers)
	}
	if start < 0 {
		return paintWindow{}, false
	}
	w = paintWindow{start: start, end: len(body)}
	if rel := earliestIndex(body[start:], firstPaintEntityMarkers); rel >= 0 {
		w.end = start + rel
	}
	return w, true
}

// contains reports whether idx lies strictly after the arm call and before
// the first entity.
func (w paintWindow) contains(idx int) bool { return idx > w.start && idx < w.end }

// text returns the window's slice of body.
func (w paintWindow) text(body string) string { return body[w.start:w.end] }

// detectFirstPaintGaps flags heavy I/O that runs ahead of evo's init call
// (FP-001: nothing is armed yet, so nothing can paint) or between init and
// the first declared Task/Group/Sequence (FP-002: armed but still blank).
// Every function that calls evo.Init is in scope — Isolated nested inits
// (previewPurge) are the pit-of-success miss, not only main/run.
func detectFirstPaintGaps(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		findings = append(findings, firstPaintGapsInBody(filename, src, fn.body, fn.offset)...)
	}
	return findings
}

func firstPaintGapsInBody(filename, src, body string, offset int) []Finding {
	w, armed := armedWindow(body)
	if !armed {
		return nil
	}
	ioIdx, ioMarker := earliestMarker(body, firstPaintIOMarkers)
	if ioIdx < 0 {
		return nil
	}
	if ioIdx < w.start {
		return []Finding{{
			RuleID:     "FP-001",
			Severity:   "warning",
			Message:    "heavy I/O runs before evo.Init/New; nothing is armed to paint within 100ms of process start",
			File:       filename,
			Line:       lineAt(src, offset+ioIdx),
			Suggestion: "call evo.Init(...) before " + ioMarker + "...)",
		}}
	}
	if !w.contains(ioIdx) {
		return nil
	}
	return []Finding{{
		RuleID:     "FP-002",
		Severity:   "warning",
		Message:    "heavy I/O runs between evo.Init/New and the first Task/Group/Sequence; declare the first entity before this I/O",
		File:       filename,
		Line:       lineAt(src, offset+ioIdx),
		Suggestion: "declare the first Task/Group/Sequence before " + ioMarker + "...)",
	}}
}

// detectSilentPreTaskLoops flags a for/range work loop that runs after
// evo.Init/New but before the first Task/Group/Sequence. That is the
// purge/prune FAIL class: scanning looks dead because the loop never
// lived inside a task definition.
func detectSilentPreTaskLoops(filename, src string) []Finding {
	var findings []Finding
	for _, fn := range allFuncBodies(src) {
		findings = append(findings, silentPreTaskLoopsInBody(filename, src, fn.body, fn.offset)...)
	}
	return findings
}

func silentPreTaskLoopsInBody(filename, src, body string, offset int) []Finding {
	w, armed := armedWindow(body)
	if !armed {
		return nil
	}
	window := w.text(body)
	loopIdx := strings.Index(window, "for ")
	if loopIdx < 0 {
		return nil
	}
	// Require range + a work marker so tiny in-memory for-loops are not flagged.
	loopTail := window[loopIdx:]
	if !strings.Contains(loopTail, " range ") {
		return nil
	}
	ioIdx, ioMarker := earliestMarker(loopTail, firstPaintIOMarkers)
	if ioIdx < 0 {
		ioIdx, ioMarker = earliestMarker(loopTail, loopWalkMarkers)
	}
	if ioIdx < 0 {
		return nil
	}
	return []Finding{{
		RuleID:     "LOOP-001",
		Severity:   "error",
		Message:    "work loop runs before any Task/Group/Sequence; silent pre-output loops are a pit-of-success FAIL — put the loop inside Task.Define, or declare Group/Sequence first and give each item its own named Task",
		File:       filename,
		Line:       lineAt(src, offset+w.start+loopIdx),
		Suggestion: "declare Task/Group/Sequence first, then run the loop inside task.Define(...) or for _, x := range items { group.Task(x).Define(...) }; move " + ioMarker + " into the task body",
	}}
}
