// Command hillclimb holds the mechanical decisions of the eval hill-climb:
//
//	hillclimb standing TRANSCRIPT...        print the training standing as JSON
//	hillclimb decide BEFORE.json AFTER.json exit 0 to keep the edit, 1 to revert
//	hillclimb guard BASE [REPO]             exit 1 if the working tree breaks a guard
//
// The worker that proposes edits is not here: see eval/hillclimb/hillclimb.sh.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zachbornheimer/evident-output/eval/hillclimb"
	"github.com/zachbornheimer/evident-output/eval/scoreboard"
)

const defaultRepo = ".."

func main() {
	ok, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "hillclimb:", err)
		os.Exit(2)
	}
	if !ok {
		os.Exit(1)
	}
}

func run(args []string) (bool, error) {
	if len(args) == 0 {
		return false, fmt.Errorf("choose a subcommand: standing, decide or guard")
	}
	switch args[0] {
	case "standing":
		return true, printStanding(args[1:])
	case "decide":
		return decide(args[1:])
	case "guard":
		return guard(args[1:])
	default:
		return false, fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func printStanding(paths []string) error {
	records, err := scoreboard.ReadRecordFiles(paths)
	if err != nil {
		return fmt.Errorf("read transcripts: %w", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(hillclimb.StandingFromRecords(records)); err != nil {
		return fmt.Errorf("print standing: %w", err)
	}
	return nil
}

func readStanding(path string) (hillclimb.Standing, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return hillclimb.Standing{}, fmt.Errorf("read standing %s: %w", path, err)
	}
	var standing hillclimb.Standing
	if err := json.Unmarshal(data, &standing); err != nil {
		return hillclimb.Standing{}, fmt.Errorf("parse standing %s: %w", path, err)
	}
	return standing, nil
}

func decide(args []string) (bool, error) {
	if len(args) != 2 {
		return false, fmt.Errorf("usage: decide BEFORE.json AFTER.json")
	}
	before, err := readStanding(args[0])
	if err != nil {
		return false, fmt.Errorf("decide: %w", err)
	}
	after, err := readStanding(args[1])
	if err != nil {
		return false, fmt.Errorf("decide: %w", err)
	}
	decision := hillclimb.Accept(before, after)
	fmt.Println(decision.Reason)
	return decision.Accepted, nil
}

func guard(args []string) (bool, error) {
	if len(args) == 0 || len(args) > 2 {
		return false, fmt.Errorf("usage: guard BASE [REPO]")
	}
	repo := gitRepo{root: defaultRepo}
	if len(args) == 2 {
		repo.root = args[1]
	}
	edit, err := repo.editSince(args[0])
	if err != nil {
		return false, fmt.Errorf("guard: %w", err)
	}
	nouns, err := repo.nounGuard(args[0])
	if err != nil {
		return false, fmt.Errorf("guard: %w", err)
	}
	violations := nouns.Check(edit)
	if len(violations) > 0 {
		fmt.Println(strings.Join(violations, "\n"))
	}
	return len(violations) == 0, nil
}
