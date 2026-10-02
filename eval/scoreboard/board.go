package scoreboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
)

// Entry is the best known result for one task.
type Entry struct {
	TaskStats
	// EvoSHA is the library commit the result was measured at.
	EvoSHA string `json:"evo_sha"`
}

// Board is the checked-in eval/scoreboard.json.
type Board struct {
	// EvoSHA and TunedSurfaceSHA identify the tree of the latest update.
	EvoSHA          string           `json:"evo_sha"`
	TunedSurfaceSHA string           `json:"tuned_surface_sha"`
	Tasks           map[string]Entry `json:"tasks"`
}

// Merge folds new stats into prev, keeping the better result per task:
// higher pass@1, then pass^k, then lower cost per pass.
func Merge(prev Board, stats []TaskStats, evoSHA, tunedSurfaceSHA string) Board {
	next := Board{EvoSHA: evoSHA, TunedSurfaceSHA: tunedSurfaceSHA, Tasks: map[string]Entry{}}
	maps.Copy(next.Tasks, prev.Tasks)
	for _, candidate := range stats {
		current, known := next.Tasks[candidate.Task]
		if !known || beats(candidate, current.TaskStats) {
			next.Tasks[candidate.Task] = Entry{TaskStats: candidate, EvoSHA: evoSHA}
		}
	}
	return next
}

func beats(candidate, current TaskStats) bool {
	switch {
	case candidate.PassAt1 != current.PassAt1:
		return candidate.PassAt1 > current.PassAt1
	case candidate.PassPowerK != current.PassPowerK:
		return candidate.PassPowerK
	case candidate.CostPerPassUSD == nil || current.CostPerPassUSD == nil:
		return current.CostPerPassUSD == nil && candidate.CostPerPassUSD != nil
	default:
		return *candidate.CostPerPassUSD < *current.CostPerPassUSD
	}
}

// Write encodes the board as indented JSON.
func (b Board) Write(out io.Writer) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(b); err != nil {
		return fmt.Errorf("write scoreboard: %w", err)
	}
	return nil
}

// ReadBoard decodes a board; an empty reader yields an empty board.
func ReadBoard(in io.Reader) (Board, error) {
	var board Board
	if err := json.NewDecoder(in).Decode(&board); err != nil && !errors.Is(err, io.EOF) {
		return Board{}, fmt.Errorf("read scoreboard: %w", err)
	}
	return board, nil
}
