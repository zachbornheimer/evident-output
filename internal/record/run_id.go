package record

import (
	"strconv"
	"time"
)

// runIDBase is the radix of every number in a run id: short, and safe in a file name or a log field.
const runIDBase = 36

// NewRunID names one run from where and when it began: the start time in nanoseconds, the process that
// ran it, and how many runs that process had already started. Two runs share an id only if one process
// starts both in the same nanosecond with the same sequence number, which the sequence rules out.
func NewRunID(started time.Time, pid int, sequence uint64) string {
	return "run_" + strconv.FormatInt(started.UnixNano(), runIDBase) +
		"_" + strconv.FormatInt(int64(pid), runIDBase) +
		"_" + strconv.FormatUint(sequence, runIDBase)
}
