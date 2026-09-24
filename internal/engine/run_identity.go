package engine

import "crypto/rand"

// runIDPrefix marks a run identity on the wire ("run_id", spec §16/§35).
const runIDPrefix = "run_"

// runIDSeqSlot is the id sequence value the run itself consumed through
// 1.1, when its identity was "out_1". Starting the sequence past it keeps
// every Task, Group, and message id on the wire where consumers found it
// (the first Task stays "task_2") now that run_id is random.
const runIDSeqSlot = 1

// newRunID is the facade every Output draws its run identity from. The
// identity is random, not a per-process counter: concurrent embedded runs
// (spec §53, one Output per HTTP request) and runs from separate processes
// must never share a run_id a machine consumer correlates on.
var newRunID = func() string { return runIDPrefix + rand.Text() }
