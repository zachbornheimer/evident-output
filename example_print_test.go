package evo

import (
	"fmt"
	"log/slog"
)

// ExampleLogRecord shows the shape journaled to Debug/Capture mirrors and
// the slog bridge: a time, level, message, and structured attrs.
func ExampleLogRecord() {
	rec := logRecord{Message: "starting up", Level: slog.LevelInfo}
	fmt.Println(rec.Message, rec.Level)
	// Output:
	// starting up INFO
}
