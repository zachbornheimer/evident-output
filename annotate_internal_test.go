package evo

import (
	"errors"
	"io"
	"testing"
)

// annotationVerbs is every non-terminal Task annotation, so the
// terminal-row guard is proven once for all of them rather than per copy.
var annotationVerbs = map[string]func(*Task){
	"Doing":    func(t *Task) { t.Doing("step") },
	"Progress": func(t *Task) { t.Progress(1, 2) },
	"Bytes":    func(t *Task) { t.bytes(1, 2) },
	"Summary":  func(t *Task) { t.Summary("ok") },
	"Problem":  func(t *Task) { t.Problem("broken") },
	"Warning":  func(t *Task) { t.Problem("careful", severity(severityWarning)) },
	"Fact":     func(t *Task) { t.Fact("path", "/tmp/x") },
}

func misuseOf(o *Run) error {
	return o.firstMisuse()
}

// TestAnnotation_AfterInterruptIsNotMisuse pins that a straggling
// annotation on a row the interrupt already cancelled is not the caller's
// misuse, for every annotation verb alike.
func TestAnnotation_AfterInterruptIsNotMisuse(t *testing.T) {
	for name, annotate := range annotationVerbs {
		t.Run(name, func(t *testing.T) {
			out := initRun(Config{plain: true, color: colorNever, stdout: io.Discard, stderr: io.Discard})
			task := out.Task("work")
			task.cancel("interrupted")
			annotate(task)
			if err := misuseOf(out); err != nil {
				t.Fatalf("%s after interrupt recorded misuse: %v", name, err)
			}
		})
	}
}

// TestAnnotation_AfterCallerResolutionIsMisuse keeps the other half of the
// rule: annotating a row the caller itself already resolved is misuse.
func TestAnnotation_AfterCallerResolutionIsMisuse(t *testing.T) {
	for name, annotate := range annotationVerbs {
		t.Run(name, func(t *testing.T) {
			out := initRun(Config{plain: true, color: colorNever, stdout: io.Discard, stderr: io.Discard})
			task := out.Task("work")
			task.fail("done badly")
			annotate(task)
			if err := misuseOf(out); !errors.Is(err, errAlreadyResolved) {
				t.Fatalf("%s after Fail recorded %v, want ErrAlreadyResolved", name, err)
			}
		})
	}
}
