package record

import "slices"

// journal is the run's durable event log, bounded under backpressure
// (CON-008): once it holds more than its cap, the oldest non-critical
// events go first, and critical ones only when nothing else is left.
//
// Retention depends only on the final sequence — every critical event
// that fits, then the newest non-critical ones — so the journal compacts
// in batches, not per event: it grows to twice its cap before one linear
// pass trims it, and a reader trims it exactly before copying. The cost
// per event stays constant however long the run.
type journal struct {
	events []Event
	// seq is the last sequence number assigned: monotonic in append order,
	// never reused after the event holding it is dropped.
	seq uint64
}

// append stamps e with the next sequence number and records it. limit <= 0
// means unbounded.
func (j *journal) append(e Event, limit int) Event {
	j.seq++
	e.Sequence = j.seq
	j.events = append(j.events, e)
	if limit > 0 && len(j.events) > 2*limit {
		j.compact(limit)
	}
	return e
}

// snapshot returns a copy of the retained events.
func (j *journal) snapshot(limit int) []Event {
	j.compact(limit)
	return slices.Clone(j.events)
}

// compact trims the journal to limit events: every critical event that
// fits (the newest, when even they overflow), then the newest
// non-critical ones.
func (j *journal) compact(limit int) {
	if limit <= 0 || len(j.events) <= limit {
		return
	}
	critical := 0
	for _, ev := range j.events {
		if criticalEventType(ev.Type) {
			critical++
		}
	}
	dropCritical := max(0, critical-limit)
	keepPlain := max(0, limit-critical)
	dropPlain := len(j.events) - critical - keepPlain
	j.events = slices.DeleteFunc(j.events, func(ev Event) bool {
		if criticalEventType(ev.Type) {
			if dropCritical > 0 {
				dropCritical--
				return true
			}
			return false
		}
		if dropPlain > 0 {
			dropPlain--
			return true
		}
		return false
	})
}

// criticalEventType reports the events never dropped under backpressure
// while anything else is left to drop (CON-008).
func criticalEventType(t string) bool {
	switch t {
	case "output.failed", "output.finished", "output.cancelled",
		"task.blocked", "task.failed", "output.started":
		return true
	default:
		return false
	}
}
