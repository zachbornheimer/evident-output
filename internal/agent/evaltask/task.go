// Package evaltask is the deterministic half of the pit-of-success eval: a
// fixed task set, a grader that scores any candidate program against a
// task, and a model-free replay of the hand-written reference answers. The
// paid model driver (a separate module) reuses the same Grader.
package evaltask

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
)

const (
	tasksDir        = "tasks"
	trapsDir        = "traps"
	promptFile      = "prompt.md"
	expectFile      = "expect.json"
	trapFile        = "trap.json"
	fixtureDir      = "fixture"
	referenceDir    = "reference"
	trapAnswerDir   = "answer"
	candidateSuffix = ".go"
)

// Task is one developer request with its hermetic fixture, expected
// topology, and hand-written reference answer.
type Task struct {
	ID     string
	Prompt string
	Expect Expect
	// Root is the task directory inside FS.
	Root string
	FS   fs.FS
}

// Expect is what a correct answer to a task must look like at run time.
type Expect struct {
	ID string `json:"id"`
	// BlockedAPI names the capabilities today's API lacks; a task with any
	// is skipped by the replay and listed in its output.
	BlockedAPI []string `json:"blocked_api,omitempty"`
	// Tree is the expected topology: top-level nodes, siblings sorted by name.
	Tree []Node `json:"tree"`
	// Order lists run-order constraints (After edges, implicit Sequence order).
	Order []OrderEdge `json:"order,omitempty"`
	// BannedPatterns names the source-pattern detectors that must not fire.
	BannedPatterns []string `json:"banned_patterns,omitempty"`
}

// OrderEdge says the Before node finished before the After node started.
type OrderEdge struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// Trap is a deliberately bad answer the eval must reject.
type Trap struct {
	ID string `json:"id"`
	// Task is the task the bad answer attempts.
	Task string `json:"task"`
	// BannedPattern is the detector that must flag the answer.
	BannedPattern string `json:"banned_pattern,omitempty"`
	// ReviewRules are review rule IDs, one of which must flag the answer
	// today. Empty when review has no rule yet.
	ReviewRules []string `json:"review_rules,omitempty"`
	// ExpectedFlagPending marks a trap review cannot flag until its rule
	// lands: the replay asserts review stays silent so landing the rule
	// forces this file to be updated.
	ExpectedFlagPending bool     `json:"expected_flag_pending,omitempty"`
	BlockedAPI          []string `json:"blocked_api,omitempty"`
	Root                string   `json:"-"`
	FS                  fs.FS    `json:"-"`
}

// Blocked reports whether the task cannot run on today's API.
func (t Task) Blocked() bool { return len(t.Expect.BlockedAPI) > 0 }

// Blocked reports whether the trap cannot run on today's API.
func (t Trap) Blocked() bool { return len(t.BlockedAPI) > 0 }

// FixtureFS is the task's stub domain package.
func (t Task) FixtureFS() (fs.FS, error) { return subFS(t.FS, path.Join(t.Root, fixtureDir)) }

// ReferenceFS is the hand-written canonical answer.
func (t Task) ReferenceFS() (fs.FS, error) { return subFS(t.FS, path.Join(t.Root, referenceDir)) }

// AnswerFS is the trap's bad answer.
func (t Trap) AnswerFS() (fs.FS, error) { return subFS(t.FS, path.Join(t.Root, trapAnswerDir)) }

func subFS(fsys fs.FS, dir string) (fs.FS, error) {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("open directory %s: %w", dir, err)
	}
	return sub, nil
}

// LoadTasks reads every task under tasks/ in fsys, in name order.
func LoadTasks(fsys fs.FS) ([]Task, error) {
	entries, err := fs.ReadDir(fsys, tasksDir)
	if err != nil {
		return nil, fmt.Errorf("list tasks in %s: %w", tasksDir, err)
	}
	var tasks []Task
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		task, err := loadTask(fsys, path.Join(tasksDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func loadTask(fsys fs.FS, root string) (Task, error) {
	task := Task{ID: path.Base(root), Root: root, FS: fsys}
	prompt, err := fs.ReadFile(fsys, path.Join(root, promptFile))
	if err != nil {
		return Task{}, fmt.Errorf("read prompt for task %s: %w", task.ID, err)
	}
	task.Prompt = string(prompt)
	if err := readJSON(fsys, path.Join(root, expectFile), &task.Expect); err != nil {
		return Task{}, fmt.Errorf("load expectation for task %s: %w", task.ID, err)
	}
	return task, nil
}

// LoadTraps reads every trap under traps/ in fsys, in name order.
func LoadTraps(fsys fs.FS) ([]Trap, error) {
	entries, err := fs.ReadDir(fsys, trapsDir)
	if err != nil {
		return nil, fmt.Errorf("list traps in %s: %w", trapsDir, err)
	}
	var traps []Trap
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := path.Join(trapsDir, entry.Name())
		trap := Trap{Root: root, FS: fsys}
		if err := readJSON(fsys, path.Join(root, trapFile), &trap); err != nil {
			return nil, fmt.Errorf("load trap %s: %w", entry.Name(), err)
		}
		traps = append(traps, trap)
	}
	return traps, nil
}

func readJSON(fsys fs.FS, name string, into any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}
