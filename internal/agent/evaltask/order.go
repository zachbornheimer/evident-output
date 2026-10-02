package evaltask

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	eventTaskDeclared = "task.declared"
	eventTaskStarted  = "task.started"
	eventTaskFinished = "task.finished"
)

type runEvent struct {
	Seq      int    `json:"seq"`
	Type     string `json:"type"`
	EntityID string `json:"entity_id"`
	Payload  struct {
		Name string `json:"name"`
	} `json:"payload"`
}

// RunOrder is when each task, by name, started and finished in an
// EVO_OUTPUT=jsonl event stream. Positions are journal sequence numbers.
type RunOrder struct {
	started  map[string]int
	finished map[string]int
}

// ParseRunOrder reads an EVO_OUTPUT=jsonl stream. The run document carries
// no After edges, so order constraints are checked against this journal.
func ParseRunOrder(stream []byte) (RunOrder, error) {
	order := RunOrder{started: map[string]int{}, finished: map[string]int{}}
	names := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(nil, len(stream)+1)
	for scanner.Scan() {
		var ev runEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return RunOrder{}, fmt.Errorf("parse evo.event line %q: %w", scanner.Text(), err)
		}
		switch ev.Type {
		case eventTaskDeclared:
			names[ev.EntityID] = ev.Payload.Name
		case eventTaskStarted:
			order.started[names[ev.EntityID]] = ev.Seq
		case eventTaskFinished:
			order.finished[names[ev.EntityID]] = ev.Seq
		}
	}
	if err := scanner.Err(); err != nil {
		return RunOrder{}, fmt.Errorf("read evo.event stream: %w", err)
	}
	return order, nil
}

// Violations returns one message per edge the run did not honor: Before must
// have finished before After started.
func (o RunOrder) Violations(edges []OrderEdge) []string {
	var out []string
	for _, edge := range edges {
		finished, haveFinished := o.finished[edge.Before]
		started, haveStarted := o.started[edge.After]
		switch {
		case !haveFinished:
			out = append(out, fmt.Sprintf("%q never finished", edge.Before))
		case !haveStarted:
			out = append(out, fmt.Sprintf("%q never started", edge.After))
		case finished > started:
			out = append(out, fmt.Sprintf("%q started before %q finished", edge.After, edge.Before))
		}
	}
	return out
}
