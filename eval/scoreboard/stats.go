// Package scoreboard turns eval transcripts into per-task pass rates, keeps
// the checked-in best-known board, and reports held-out results as
// aggregates only.
package scoreboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/zachbornheimer/evident-output/eval/runner"
)

const maxTranscriptLineBytes = 64 << 20

// TaskStats is how one model did on one task across its k samples.
type TaskStats struct {
	Task    string `json:"task"`
	Model   string `json:"model"`
	Samples int    `json:"samples"`
	Passes  int    `json:"passes"`
	// PassAt1 is the fraction of samples that passed.
	PassAt1 float64 `json:"pass_at_1"`
	// PassPowerK is true when all k samples passed (pass^k).
	PassPowerK bool `json:"pass_power_k"`
	// MeanCyclesToClean averages review cycles over samples that reached a
	// clean review; nil when none did.
	MeanCyclesToClean *float64 `json:"mean_cycles_to_clean"`
	// CostPerPassUSD is total spend over passes; nil when nothing passed.
	CostPerPassUSD *float64 `json:"cost_per_pass_usd"`
}

type taskKey struct{ task, model string }

type tally struct {
	samples, passes, cleaned int
	cycles                   int
	costUSD                  float64
}

// Compute groups records by task and model, in name order.
func Compute(records []runner.Record) []TaskStats {
	tallies := map[taskKey]*tally{}
	for _, record := range records {
		key := taskKey{record.Task, record.Model}
		if tallies[key] == nil {
			tallies[key] = &tally{}
		}
		tallies[key].add(record)
	}
	stats := make([]TaskStats, 0, len(tallies))
	for key, t := range tallies {
		stats = append(stats, t.stats(key))
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Task != stats[j].Task {
			return stats[i].Task < stats[j].Task
		}
		return stats[i].Model < stats[j].Model
	})
	return stats
}

func (t *tally) add(record runner.Record) {
	t.samples++
	t.costUSD += record.CostUSD
	if record.Passed {
		t.passes++
	}
	if record.CyclesToClean >= 0 {
		t.cleaned++
		t.cycles += record.CyclesToClean
	}
}

func (t *tally) stats(key taskKey) TaskStats {
	out := TaskStats{
		Task: key.task, Model: key.model, Samples: t.samples, Passes: t.passes,
		PassAt1:    float64(t.passes) / float64(t.samples),
		PassPowerK: t.passes == t.samples,
	}
	if t.cleaned > 0 {
		mean := float64(t.cycles) / float64(t.cleaned)
		out.MeanCyclesToClean = &mean
	}
	if t.passes > 0 {
		perPass := t.costUSD / float64(t.passes)
		out.CostPerPassUSD = &perPass
	}
	return out
}

// ReadRecords parses a JSONL transcript.
func ReadRecords(in io.Reader) ([]runner.Record, error) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(nil, maxTranscriptLineBytes)
	var records []runner.Record
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var record runner.Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("parse transcript line %d: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	return records, nil
}
