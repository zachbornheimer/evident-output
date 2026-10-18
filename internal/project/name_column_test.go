package project_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestNameColumnIgnoresEarlierConfirmRow pins that sibling rows align to each
// other and not to a Confirm question: the question is a prompt with its own
// answer, so a long one never pads the rows that follow it.
func TestNameColumnIgnoresEarlierConfirmRow(t *testing.T) {
	var stderr bytes.Buffer
	evo.Execute(context.Background(), evo.Config{
		Name: "column", Output: evo.Plain,
		Facades: evo.Facades{Stdout: &bytes.Buffer{}, Stderr: &stderr, Stdin: strings.NewReader("")},
	}, func(_ context.Context, r *evo.Run) error {
		publish := r.Task("publish").Define(func(context.Context) error {
			if !r.Confirm("publish the release to production?", evo.NonInteractive("--yes")) {
				return evo.Refuse("declined")
			}
			return nil
		})
		r.Task("audit").After(publish).Define(func(context.Context) error { return nil })
		return nil
	})

	want := "⊘ publish the release to production?  no terminal to answer\n" +
		"⊘ publish  declined\n" +
		"- audit    not started — publish refused\n"
	if got := stderr.String(); !strings.HasPrefix(got, want) {
		t.Errorf("rows =\n%s\nwant them to start\n%s", got, want)
	}
}
