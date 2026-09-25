package engine

import (
	"errors"
	"io"
	"testing"
)

// annotationVerbs is every non-terminal TaskHandle annotation, so the
// terminal-row guard is proven once for all of them rather than per copy.
var annotationVerbs = map[string]func(*TaskHandle){
	"Doing":    func(t *TaskHandle) { t.Doing("step") },
	"Progress": func(t *TaskHandle) { t.Progress(1, 2) },
	"Bytes":    func(t *TaskHandle) { t.Bytes(1, 2) },
	"Summary":  func(t *TaskHandle) { t.Summary("ok") },
	"Warn":     func(t *TaskHandle) { t.Warn("careful") },
	"Problem":  func(t *TaskHandle) { t.Problem("broken") },
	"Fact":     func(t *TaskHandle) { t.Fact("path", "/tmp/x") },
}

func misuseOf(o *Output) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.misuse
}

// TestAnnotation_AfterInterruptIsNotMisuse pins that a straggling
// annotation on a row the interrupt already cancelled is not the caller's
// misuse, for every annotation verb alike.
func TestAnnotation_AfterInterruptIsNotMisuse(t *testing.T) {
	for name, annotate := range annotationVerbs {
		t.Run(name, func(t *testing.T) {
			out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: io.Discard, Stderr: io.Discard})
			task := out.Task("work")
			task.Cancel("interrupted")
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
			out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: io.Discard, Stderr: io.Discard})
			task := out.Task("work")
			task.Fail("done badly")
			annotate(task)
			if err := misuseOf(out); !errors.Is(err, ErrAlreadyResolved) {
				t.Fatalf("%s after Fail recorded %v, want ErrAlreadyResolved", name, err)
			}
		})
	}
}
