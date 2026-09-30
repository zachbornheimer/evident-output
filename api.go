package evo

import (
	"io"
	"log/slog"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) { engine.Print(args...) }

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) { engine.Printf(format, args...) }

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) { engine.Println(args...) }

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer { return wrapPrinter(engine.Verbose()) }

// SlogHandler returns a slog.Handler journaling to the default instance.
func SlogHandler() slog.Handler { return engine.SlogHandler() }

// Confirm asks question on the default instance and returns whether the user accepted.
func Confirm(question string, opts ...ConfirmOption) bool {
	return engine.Confirm(question, opts...)
}

func AssumeYes(v bool) ConfirmOption              { return engine.AssumeYes(v) }
func ConfirmDetail(lines ...string) ConfirmOption { return engine.ConfirmDetail(lines...) }
func Destructive() ConfirmOption                  { return engine.Destructive() }
func PolicyFlag(flag string) ConfirmOption        { return engine.PolicyFlag(flag) }
func PolicyHint(command string, args ...string) ConfirmOption {
	return engine.PolicyHint(command, args...)
}

func IsCharDevice(w io.Writer) bool { return engine.IsCharDevice(w) }
func Pluralize(quantity int64, singular string) string {
	return engine.Pluralize(quantity, singular)
}
func TruncateNames(names []string, visible int) string {
	return engine.TruncateNames(names, visible)
}

func NewestFirst() DebugPaneOption         { return engine.NewestFirst() }
func OldestFirst() DebugPaneOption         { return engine.OldestFirst() }
func PaneHeight(lines int) DebugPaneOption { return engine.PaneHeight(lines) }
func PreserveDebugTail() DebugPaneOption   { return engine.PreserveDebugTail() }
