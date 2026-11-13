package evo

import (
	"iter"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func eachChildren(out *Output, groupID string, items []string) iter.Seq2[string, *TaskHandle] {
	return func(yield func(string, *TaskHandle) bool) {
		if out == nil {
			return
		}
		var wait []*TaskHandle
		for _, item := range items {
			task := out.addEachChild(groupID, item)
			cont := yield(item, task)
			if task.wasSubmitted() {
				wait = append(wait, task)
			}
			if !cont {
				break
			}
		}
		for _, task := range wait {
			task.waitSubmitted()
		}
	}
}

func (o *Output) addEachChild(groupID, name string) *TaskHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[groupID]
	if col == nil {
		return &TaskHandle{out: o, id: o.nextID("task")}
	}
	return o.addTaskLocked(txt.Text(name), col, "", true)
}
