package graph

import (
	"bytes"
	"runtime"
	"runtime/metrics"
	"strconv"
)

// GoroutineID names one goroutine as the runtime prints it in a traceback.
// Zero means unknown. Go exposes no goroutine identity, and Wait takes no
// context, so the traceback header is the only place "which goroutine is
// this, and which goroutine started it" can be read from.
type GoroutineID uint64

// goroutineHeaderBytes holds the traceback's first line,
// "goroutine 123 [running]:", with room to spare.
const goroutineHeaderBytes = 64

// goroutineTraceBytes is room for one goroutine's whole traceback: a parked
// goroutine prints a few hundred bytes, a callback several frames more.
const goroutineTraceBytes = 2048

var (
	goroutinePrefix = []byte("goroutine ")
	creatorMarker   = []byte(" in goroutine ")
	// goroutineSeparator is the blank line between two goroutines' tracebacks.
	goroutineSeparator = []byte("\n\n")
)

// CurrentGoroutine is the calling goroutine's id, read from the header of
// its own traceback.
func CurrentGoroutine() GoroutineID {
	var buf [goroutineHeaderBytes]byte
	return parseGoroutineHeader(buf[:runtime.Stack(buf[:], false)])
}

// CurrentGoroutineLineage is the calling goroutine's id and the id of the
// goroutine that started it (zero for one the runtime started). It reads
// the whole traceback, because the creator is named on its last line.
func CurrentGoroutineLineage() (self, creator GoroutineID) {
	for size := 4 * goroutineHeaderBytes; ; size *= 2 {
		buf := make([]byte, size)
		n := runtime.Stack(buf, false)
		if n < size {
			trace := buf[:n]
			return parseGoroutineHeader(trace), parseGoroutineCreator(trace)
		}
	}
}

// createdMetric counts every goroutine the process ever started.
const createdMetric = "/sched/goroutines-created:goroutines"

// goroutinesCreated is how many goroutines the process has started so far, and
// false on a toolchain that does not count them. It never stops the world.
func goroutinesCreated() (uint64, bool) {
	sample := [1]metrics.Sample{{Name: createdMetric}}
	metrics.Read(sample[:])
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0, false
	}
	return sample[0].Value.Uint64(), true
}

// goroutinesStartedBy is every goroutine alive now that root started,
// directly or through others. It is read from the last dump of every
// goroutine while the process has started none since, and from a fresh one
// otherwise.
func goroutinesStartedBy(root GoroutineID) startedGoroutines {
	return processLineage.descendantsOf(root)
}

// goroutineCreators maps every goroutine in a dump of all tracebacks to the
// goroutine that started it (zero for one the runtime started).
func goroutineCreators(dump []byte) map[GoroutineID]GoroutineID {
	creators := make(map[GoroutineID]GoroutineID)
	for trace := range bytes.SplitSeq(dump, goroutineSeparator) {
		if id := parseGoroutineHeader(trace); id != 0 {
			creators[id] = parseGoroutineCreator(trace)
		}
	}
	return creators
}

// allGoroutineTraces is every goroutine's traceback, the calling goroutine's
// first, one blank line between them. Each attempt stops the world, so the
// buffer starts sized for the goroutines alive now and a typical dump takes
// one attempt.
func allGoroutineTraces() []byte {
	for size := (runtime.NumGoroutine() + 1) * goroutineTraceBytes; ; size *= 2 {
		buf := make([]byte, size)
		if n := runtime.Stack(buf, true); n < size {
			return buf[:n]
		}
	}
}

// parseGoroutineHeader reads N from a traceback starting "goroutine N [".
func parseGoroutineHeader(trace []byte) GoroutineID {
	rest, ok := bytes.CutPrefix(trace, goroutinePrefix)
	if !ok {
		return 0
	}
	return parseGoroutineNumber(rest)
}

// parseGoroutineCreator reads N from the traceback's closing
// "created by F in goroutine N" line.
func parseGoroutineCreator(trace []byte) GoroutineID {
	at := bytes.LastIndex(trace, creatorMarker)
	if at < 0 {
		return 0
	}
	return parseGoroutineNumber(trace[at+len(creatorMarker):])
}

func parseGoroutineNumber(b []byte) GoroutineID {
	end := 0
	for end < len(b) && b[end] >= '0' && b[end] <= '9' {
		end++
	}
	id, err := strconv.ParseUint(string(b[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return GoroutineID(id)
}
