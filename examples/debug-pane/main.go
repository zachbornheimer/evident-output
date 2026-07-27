// Command debug-pane demos v0.4 DebugPane: rolling slog-text viewport in the
// live region (newest first). On success the pane is removed; on failure a
// labeled diagnostic tail is preserved under the final report.
//
//	go run ./examples/debug-pane/              # success — pane gone after Finish
//	go run ./examples/debug-pane/ --fail       # failed — diagnostics tail kept
//	go run ./examples/debug-pane/ --preserve   # always keep tail (opt-in)
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/examples/internal/demo"
)

func main() {
	fast := flag.Bool("fast", false, "shorter sleeps")
	fail := flag.Bool("fail", false, "fail an item so the diagnostic tail is preserved")
	preserve := flag.Bool("preserve", false, "PreserveDebugTail even on success")
	colorFlag := flag.String("color", "auto", "auto|always|never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: debug-pane [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Pane-mode debug: a bounded viewport under tasks/items shows\n")
		fmt.Fprintf(os.Stderr, "newest-first slog text (time= level= msg= …). Not durable\n")
		fmt.Fprintf(os.Stderr, "scrollback unless Finish preserves a diagnostics tail.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	step := 100 * time.Millisecond
	if *fast {
		step = 20 * time.Millisecond
	}

	paneOpts := []evo.DebugPaneOption{
		evo.PaneHeight(4),
		evo.NewestFirst(),
	}
	if *preserve {
		paneOpts = append(paneOpts, evo.PreserveDebugTail())
	}

	out := evo.For("branch-audit",
		demo.Options(os.Stderr, demo.ParseColorFlag(*colorFlag),
			evo.DebugLevel(evo.Debug),
			evo.DebugPane(paneOpts...),
		)...,
	)
	defer out.Close()

	jobs := out.Tasks("audit")
	scan := jobs.Task("scan")
	compare := jobs.Task("compare")

	scan.Phase("enumerating")
	out.Debug("enumerated local branches", evo.Int("count", 7))
	time.Sleep(step)
	out.Debug("fetched remote metadata", evo.String("remote", "origin"))
	time.Sleep(step)
	scan.Donef("%d branches", 7)

	compare.Phase("diffing")
	out.Debug("branch comparison completed", evo.Int("blockers", 1))
	time.Sleep(step)
	out.Debug("policy check", evo.String("rule", "no-local-only"))
	time.Sleep(step)
	if *fail {
		compare.Fail("local-only branches remain")
		out.Item("branches").Block("feat/sdk-full-consolidation local-only")
	} else {
		compare.Done()
		out.Item("branches").OK()
	}

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, "presentation error:", err)
		os.Exit(1)
	}
	os.Exit(out.Conclusion().ExitCode)
}
