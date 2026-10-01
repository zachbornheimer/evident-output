// Command scoreboard folds eval transcripts into eval/scoreboard.json, or
// prints a held-out aggregate that reveals no task text.
//
//	scoreboard update --evo-sha SHA --tuned-sha SHA [--out eval/scoreboard.json] TRANSCRIPT...
//	scoreboard held-out TRANSCRIPT...
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/zachbornheimer/evident-output/eval/runner"
	"github.com/zachbornheimer/evident-output/eval/scoreboard"
)

const (
	subcommandUpdate  = "update"
	subcommandHeldOut = "held-out"
	boardFileMode     = 0o644
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scoreboard:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("choose a subcommand: %s or %s", subcommandUpdate, subcommandHeldOut)
	}
	switch args[0] {
	case subcommandUpdate:
		return update(args[1:])
	case subcommandHeldOut:
		return heldOut(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q: want %s or %s", args[0], subcommandUpdate, subcommandHeldOut)
	}
}

func readAll(paths []string) ([]runner.Record, error) {
	var all []runner.Record
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open transcript %s: %w", path, err)
		}
		records, err := scoreboard.ReadRecords(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("read transcript %s: %w", path, err)
		}
		all = append(all, records...)
	}
	return all, nil
}

func heldOut(args []string) error {
	records, err := readAll(args)
	if err != nil {
		return fmt.Errorf("held-out report: %w", err)
	}
	fmt.Println(scoreboard.HeldOut(records))
	return nil
}

func update(args []string) error {
	flags := flag.NewFlagSet(subcommandUpdate, flag.ContinueOnError)
	evoSHA := flags.String("evo-sha", "", "library commit the transcripts were measured at")
	tunedSHA := flags.String("tuned-sha", "", "commit of the tuned surface")
	out := flags.String("out", "scoreboard.json", "scoreboard file to update")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *evoSHA == "" || *tunedSHA == "" {
		return errors.New("--evo-sha and --tuned-sha are required")
	}
	records, err := readAll(flags.Args())
	if err != nil {
		return fmt.Errorf("update scoreboard: %w", err)
	}
	prev, err := readBoardFile(*out)
	if err != nil {
		return fmt.Errorf("update scoreboard: %w", err)
	}
	next := scoreboard.Merge(prev, scoreboard.Compute(records), *evoSHA, *tunedSHA)
	file, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, boardFileMode)
	if err != nil {
		return fmt.Errorf("open %s: %w", *out, err)
	}
	defer file.Close()
	return next.Write(file)
}

func readBoardFile(path string) (scoreboard.Board, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return scoreboard.Board{}, nil
	}
	if err != nil {
		return scoreboard.Board{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	board, err := scoreboard.ReadBoard(file)
	if err != nil {
		return scoreboard.Board{}, fmt.Errorf("read %s: %w", path, err)
	}
	return board, nil
}
