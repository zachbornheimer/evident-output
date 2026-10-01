package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

const (
	resultsDateLayout = "2006-01-02"
	shaPrefixLength   = 12
)

// Record is one sample's line in the JSONL transcript.
type Record struct {
	Task          string            `json:"task"`
	Model         string            `json:"model"`
	Sample        int               `json:"sample"`
	Turns         int               `json:"turns"`
	ToolCalls     int               `json:"tool_calls"`
	Files         map[string]string `json:"files,omitempty"`
	Ended         string            `json:"ended"`
	Grade         *evaltask.Report  `json:"grade,omitempty"`
	Passed        bool              `json:"passed"`
	CyclesToClean int               `json:"cycles_to_clean"`
	Tokens        driver.Usage      `json:"tokens"`
	CostUSD       float64           `json:"cost_usd"`
	Error         string            `json:"error,omitempty"`
}

// Sink receives records as samples finish.
type Sink interface {
	Write(Record) error
}

// JSONLSink writes one JSON object per line.
type JSONLSink struct{ Out io.Writer }

// Write appends one record.
func (s JSONLSink) Write(record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode transcript for task %s sample %d: %w", record.Task, record.Sample, err)
	}
	if _, err := s.Out.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write transcript for task %s sample %d: %w", record.Task, record.Sample, err)
	}
	return nil
}

// ResultsPath is <dir>/<date>-<sha>-<model>.jsonl.
func ResultsPath(dir string, now time.Time, evoSHA, model string) string {
	sha := evoSHA
	if len(sha) > shaPrefixLength {
		sha = sha[:shaPrefixLength]
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s.jsonl", now.Format(resultsDateLayout), sha, model))
}
