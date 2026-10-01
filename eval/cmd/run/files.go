package main

import (
	"fmt"
	"os"
	"time"

	"github.com/zachbornheimer/evident-output/eval/runner"
)

const transcriptFileMode = 0o644

// transcriptFiles opens one JSONL transcript per model and closes them at
// the end of the run.
type transcriptFiles struct {
	dir  string
	sha  string
	now  time.Time
	open []*os.File
}

func (t *transcriptFiles) sinkFor(model string) (runner.Sink, error) {
	path := runner.ResultsPath(t.dir, t.now, t.sha, model)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, transcriptFileMode)
	if err != nil {
		return nil, fmt.Errorf("open transcript %s: %w", path, err)
	}
	t.open = append(t.open, file)
	return runner.JSONLSink{Out: file}, nil
}

func (t *transcriptFiles) closeAll() {
	for _, file := range t.open {
		_ = file.Close()
	}
}
