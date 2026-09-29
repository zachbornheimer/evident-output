package engine

import (
	"bytes"
	"runtime"
	"strconv"
)

// goroutineID names one goroutine as the runtime prints it in a traceback.
// Zero means unknown. Go exposes no goroutine identity, and Wait takes no
// context, so the traceback header is the only place "which goroutine is
// this, and which goroutine started it" can be read from.
type goroutineID uint64

// goroutineHeaderBytes holds the traceback's first line,
// "goroutine 123 [running]:", with room to spare.
const goroutineHeaderBytes = 64

var (
	goroutinePrefix = []byte("goroutine ")
	creatorMarker   = []byte(" in goroutine ")
)

// currentGoroutine is the calling goroutine's id, read from the header of
// its own traceback.
func currentGoroutine() goroutineID {
	var buf [goroutineHeaderBytes]byte
	return parseGoroutineHeader(buf[:runtime.Stack(buf[:], false)])
}

// currentGoroutineLineage is the calling goroutine's id and the id of the
// goroutine that started it (zero for one the runtime started). It reads
// the whole traceback, because the creator is named on its last line.
func currentGoroutineLineage() (self, creator goroutineID) {
	for size := 4 * goroutineHeaderBytes; ; size *= 2 {
		buf := make([]byte, size)
		n := runtime.Stack(buf, false)
		if n < size {
			trace := buf[:n]
			return parseGoroutineHeader(trace), parseGoroutineCreator(trace)
		}
	}
}

// parseGoroutineHeader reads N from a traceback starting "goroutine N [".
func parseGoroutineHeader(trace []byte) goroutineID {
	rest, ok := bytes.CutPrefix(trace, goroutinePrefix)
	if !ok {
		return 0
	}
	return parseGoroutineNumber(rest)
}

// parseGoroutineCreator reads N from the traceback's closing
// "created by F in goroutine N" line.
func parseGoroutineCreator(trace []byte) goroutineID {
	at := bytes.LastIndex(trace, creatorMarker)
	if at < 0 {
		return 0
	}
	return parseGoroutineNumber(trace[at+len(creatorMarker):])
}

func parseGoroutineNumber(b []byte) goroutineID {
	end := 0
	for end < len(b) && b[end] >= '0' && b[end] <= '9' {
		end++
	}
	id, err := strconv.ParseUint(string(b[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return goroutineID(id)
}
