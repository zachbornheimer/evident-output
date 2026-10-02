package outcomes_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

type maskRedactor struct{}

func (maskRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, "SECRET", "[redacted]")
}

func TestC02_016_WriterRetainsOnlyABoundedRedactedTail(t *testing.T) {
	const lines = 500
	out, buf := harness.New(t, func(c *evo.Config) { c.Redactor = maskRedactor{} })
	task := out.Task("child")
	task.Define(func(context.Context) error {
		w := task.Writer()
		for i := range lines {
			_, _ = fmt.Fprintf(w, "line %d SECRET\n", i)
		}
		task.Fail("child failed", task.Capture().DetailTail())
		return nil
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	retained := text
	if !strings.Contains(retained, "line 499 [redacted]") {
		t.Fatalf("the redacted tail was not retained: %q", retained)
	}
	if strings.Contains(retained, "line 0 ") || strings.Count(retained, "line ") >= lines {
		t.Fatalf("tail is not bounded: %d bytes", len(retained))
	}
}
