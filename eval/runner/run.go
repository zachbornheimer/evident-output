package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

// Runner wires the driver, the grader, and the transcript sinks.
type Runner struct {
	Grader   evaltask.Grader
	Server   driver.ToolServer
	Cache    driver.BlobStore
	NewModel func(modelID string) driver.Model
	WorkDir  driver.WorkDirMaker
	SinkFor  func(modelID string) (Sink, error)
}

// Summary is the run's totals.
type Summary struct {
	Samples  int
	Passed   int
	SpentUSD float64
	Aborted  bool
}

// modelRun is everything one model's pass over the tasks needs.
type modelRun struct {
	samples int
	tasks   []evaltask.Task
	modelID string
	system  string
	meter   driver.Meter
	sink    Sink
}

// Run executes every task x model x sample. When accumulated cost reaches
// cfg.MaxUSD it stops the whole run and returns the summary so far with an
// error wrapping driver.ErrSpendCapReached.
func (r Runner) Run(ctx context.Context, cfg Config, tasks []evaltask.Task) (Summary, error) {
	if err := cfg.Validate(); err != nil {
		return Summary{}, fmt.Errorf("validate run config: %w", err)
	}
	guard, err := driver.NewSpendGuard(cfg.MaxUSD)
	if err != nil {
		return Summary{}, fmt.Errorf("start run: %w", err)
	}
	system, err := driver.LoadCorpus(ctx, r.Server, r.Cache)
	if err != nil {
		return Summary{}, fmt.Errorf("load docs corpus: %w", err)
	}
	var summary Summary
	for _, modelID := range cfg.Models {
		price, err := cfg.Prices.Resolve(modelID)
		if err != nil {
			return summary, fmt.Errorf("%w: %w (%s)", ErrRefusedToStart, err, FlagPriceMissing)
		}
		sink, err := r.SinkFor(modelID)
		if err != nil {
			return summary, fmt.Errorf("open transcript for model %s: %w", modelID, err)
		}
		run := modelRun{
			samples: cfg.Samples, tasks: tasks, modelID: modelID, system: system,
			meter: driver.Meter{Price: price, Guard: guard}, sink: sink,
		}
		err = r.runModel(ctx, run, &summary)
		summary.SpentUSD = guard.Spent()
		if err != nil {
			summary.Aborted = errors.Is(err, driver.ErrSpendCapReached)
			return summary, fmt.Errorf("run model %s: %w", modelID, err)
		}
	}
	return summary, nil
}

func (r Runner) runModel(ctx context.Context, run modelRun, summary *Summary) error {
	model := r.NewModel(run.modelID)
	for _, task := range run.tasks {
		sample, err := r.sampleFor(task, model, run)
		if err != nil {
			return fmt.Errorf("prepare task %s: %w", task.ID, err)
		}
		for index := 1; index <= run.samples; index++ {
			record, runErr := r.runSample(ctx, task, sample, index)
			record.Model = run.modelID
			summary.Samples++
			if record.Passed {
				summary.Passed++
			}
			if err := run.sink.Write(record); err != nil {
				return fmt.Errorf("record task %s sample %d: %w", task.ID, index, err)
			}
			if runErr != nil {
				return fmt.Errorf("task %s sample %d: %w", task.ID, index, runErr)
			}
		}
	}
	return nil
}

func (r Runner) sampleFor(task evaltask.Task, model driver.Model, run modelRun) (driver.Sample, error) {
	fixture, err := task.FixtureFS()
	if err != nil {
		return driver.Sample{}, fmt.Errorf("open fixture: %w", err)
	}
	api, err := driver.FixtureAPISummary(fixture)
	if err != nil {
		return driver.Sample{}, fmt.Errorf("summarize fixture: %w", err)
	}
	builder := driver.Builder{Grader: r.Grader, Task: task, WorkDir: r.WorkDir}
	return driver.Sample{
		Model: model, ModelID: run.modelID, MaxTokens: driver.DefaultMaxTokens, System: run.system,
		Prompt: task.Prompt + "\n\nFixture package API:\n\n" + api,
		Tools:  driver.Toolbox{Server: r.Server, Builder: builder},
		Meter:  run.meter,
	}, nil
}

// runSample returns the record to write and, separately, a fatal error. An
// infrastructure fault or the spend cap still yields a record of what was
// observed.
func (r Runner) runSample(ctx context.Context, task evaltask.Task, sample driver.Sample, index int) (Record, error) {
	result, err := sample.Run(ctx)
	record := Record{
		Task: task.ID, Sample: index, Turns: result.Turns, ToolCalls: result.ToolCalls, Files: result.Files,
		Ended: result.Ended, CyclesToClean: result.CyclesToClean, Tokens: result.Usage, CostUSD: result.CostUSD,
	}
	if err != nil {
		record.Error = err.Error()
		return record, err
	}
	if !result.Submitted {
		return record, nil
	}
	report, err := r.grade(ctx, task, result.Files)
	if err != nil {
		record.Error = err.Error()
		return record, err
	}
	record.Grade, record.Passed = &report, report.Passed()
	return record, nil
}

func (r Runner) grade(ctx context.Context, task evaltask.Task, files map[string]string) (evaltask.Report, error) {
	dir, cleanup, err := r.WorkDir()
	if err != nil {
		return evaltask.Report{}, fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	defer cleanup()
	report, err := r.Grader.Grade(ctx, task, driver.FileSystem(files), dir)
	if err != nil {
		return evaltask.Report{}, fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	return report, nil
}
