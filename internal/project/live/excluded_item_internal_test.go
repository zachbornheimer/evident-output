package live

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// excludedItem is a disposition item: a Task Excluded for reason, with the
// Summary and Problem Exclude records.
func excludedItem(name, reason string) record.TaskSnapshot {
	return record.TaskSnapshot{
		Name: name, State: record.Excluded, Summary: reason,
		Problems: []record.Problem{{Summary: reason}},
	}
}
