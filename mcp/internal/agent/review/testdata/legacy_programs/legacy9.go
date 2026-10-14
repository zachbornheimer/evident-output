// legacy9: a v1.0.0 program (gate 9 review 1) using surfaces neither legacy
// fixture covers: an *Output from Init with Config.Subject/Stdout, out.Run,
// out.Sequence, Verify, Writer, File with Basis, Confirm with
// AssumeYes/ConfirmDetail, Kept/Reason, Create, Push, Block with Location and
// Count, Group.Summary, Next with Command, out.Warn, out.ResultWriter. The
// Verify and Create/Push callbacks do real work, so the 2.0 program has no
// finding of its own.
//
// Each author-step is the one edit review cannot make, because the new text
// is the author's: OLD => NEW, applied to the rewritten program.
// author-step: r *evo.Run) error { => r *evo.Run) error { r.Fact("subject", "~/repo")
// author-step: pinned.Kept(evo.Reason("pinned")) => pinned.Verify(func(ctx context.Context) (bool, error) { _, err := os.Stat("v0.9.tag"); return err == nil, nil }); pinned.Define(func(ctx context.Context) error { return exec.CommandContext(ctx, "git", "tag", "v0.9").Run() })
// author-step: push.Next(evo.Command("git", "push", "--tags")) =>
// author-step: evo.On("NOTES.md:1:1"), evo.Detail("2 tag")) => evo.On("NOTES.md:1:1"), evo.Detail("2 tag"), evo.Next(evo.Action{Command: []string{"git", "push", "--tags"}}))
// author-step: out.Warn("signing skipped") => push.Problem("signing skipped")
package main

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.Init(evo.Config{Title: "release notes", Subject: "~/repo", Stdout: os.Stderr})
	res := out.Run(context.Background(), func(ctx context.Context) error {
		pipe := out.Sequence("publish")
		gen := pipe.Task("render notes")
		gen.Verify(func(context.Context) (bool, error) {
			_, err := os.Stat("NOTES.md")
			return err == nil, nil
		})
		gen.Define(func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, "true")
			cmd.Stdout = gen.Writer()
			if err := cmd.Run(); err != nil {
				return err
			}
			return evo.File(ctx, evo.FileSpec{Path: "NOTES.md", Contents: []byte("notes\n"), Basis: []evo.Fingerprint{evo.FSPath("CHANGELOG.md")}})
		})
		tags := out.Group("tags").Summary("release tags")
		pinned := tags.Task("v0.9")
		pinned.Kept(evo.Reason("pinned"))
		tags.Task("v1.0").Create("tag", func() error { return exec.Command("git", "tag", "v1.0").Run() })
		push := pipe.Task("push")
		if !out.Confirm("push tags?", evo.AssumeYes(len(os.Args) > 1), evo.ConfirmDetail("2 tags")) {
			push.Block("push declined", evo.Location("NOTES.md", 1, 1), evo.Count(2, "tag"))
			push.Next(evo.Command("git", "push", "--tags"))
			return nil
		}
		push.Push("tag", func() error { return exec.Command("git", "push", "--tags").Run() })
		out.Warn("signing skipped")
		_, err := out.ResultWriter().Write([]byte("v1.0\n"))
		return err
	})
	os.Exit(res.Conclusion.ExitCode)
}
