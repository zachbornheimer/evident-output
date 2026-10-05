package hillclimb_test

import "github.com/zachbornheimer/evident-output/eval/runner"

func recordsFor(task string, outcomes ...bool) []runner.Record {
	var records []runner.Record
	for i, passed := range outcomes {
		records = append(records, runner.Record{Task: task, Sample: i + 1, Passed: passed})
	}
	return records
}
