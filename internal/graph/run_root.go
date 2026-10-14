// This file owns the run's root node: what the run says about itself rather
// than about one Task. Every run-scoped record write goes through it.

package graph

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// RunRoot is the run's own node: its Facts and warnings, the messages and
// history lines it printed, its debug records and journal, and the
// Conclusion.
type RunRoot struct{ run *record.Run }

// Root is the run's root node.
func (g *Graph) Root() RunRoot { return RunRoot{run: g.run} }

// Note records a run-scoped Fact.
func (r RunRoot) Note(f record.Fact) { r.run.RecordRunFact(f) }

// Warn records a run-scoped warning.
func (r RunRoot) Warn(p record.Problem) { r.run.RecordRunWarning(p) }

// Print records one printed message.
func (r RunRoot) Print(m record.MessageSnapshot) { r.run.AppendMessage(m) }

// Log records one durable history line and returns how many there are.
func (r RunRoot) Log(line string) int { return r.run.AppendLine(line) }

// Debug records one debug record.
func (r RunRoot) Debug(d record.DebugRecord) { r.run.AppendDebugRecord(d) }

// Journal stamps e into the run's journal, which keeps at most limit
// events, and returns it as stamped.
func (r RunRoot) Journal(e record.Event, limit int) record.Event { return r.run.AppendEvent(e, limit) }

// Conclude records the run's Conclusion.
func (r RunRoot) Conclude(c record.Conclusion) { r.run.RecordConclusion(c) }
