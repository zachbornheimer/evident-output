// Package review — API-072 and the product-gap event (ZYS-1369). When the
// source needs an evo shape the pinned release cannot express, review has
// no honest rewrite: it keeps recheck_required=true, emits one GapEvent per
// distinct gap through a GapSink, and suggests nothing, so no local
// workaround gets invented for a missing product feature. Wiring a sink to
// an issue tracker is the caller's job; review ships the interface, the
// event, the dedupe, and an in-memory sink.
package review

import (
	"go/ast"
	"reflect"
	"sync"
)

// GapEvent is one product gap: a canonical shape the pinned release lacks.
type GapEvent struct {
	// Signature is stable for the same shape at the same pin, so a sink can
	// file the gap once however many files or reviews hit it.
	Signature       string `json:"signature"`
	Shape           string `json:"shape"`
	DesiredVersion  string `json:"desired_version"`
	RequiredVersion string `json:"required_version"`
}

// GapSink receives product-gap events.
type GapSink interface {
	Emit(GapEvent)
}

// MemoryGapSink records emitted events in order; safe for concurrent use.
type MemoryGapSink struct {
	mu     sync.Mutex
	events []GapEvent
}

// Emit records e.
func (s *MemoryGapSink) Emit(e GapEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

// Events returns a copy of the events recorded so far.
func (s *MemoryGapSink) Events() []GapEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]GapEvent(nil), s.events...)
}

// gapShape is a canonical evo shape and the first release that spells it.
type gapShape struct {
	name       string
	minVersion string
	usedIn     func(file *ast.File, evoPkg string) bool
}

// gapShapes is every shape review can tell a pin is too old for.
var gapShapes = []gapShape{
	{name: "typed dataflow (evo.Compute)", minVersion: dialectCompute, usedIn: callsEvoCompute},
}

func callsEvoCompute(file *ast.File, evoPkg string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && calledFuncDotted(call) == evoPkg+".Compute" {
			found = true
		}
		return !found
	})
	return found
}

// inexpressibleShapes are the shapes file uses that desiredVersion lacks.
// An empty pin is the current release and never has a gap.
func inexpressibleShapes(file *ast.File, desiredVersion string) []GapEvent {
	evoPkg := evoImportName(file)
	if evoPkg == "" || desiredVersion == "" {
		return nil
	}
	var events []GapEvent
	for _, shape := range gapShapes {
		if dialectAtLeast(desiredVersion, shape.minVersion) || !shape.usedIn(file, evoPkg) {
			continue
		}
		events = append(events, GapEvent{
			Signature:       shape.name + " needs " + shape.minVersion + " at pin " + desiredVersion,
			Shape:           shape.name,
			DesiredVersion:  desiredVersion,
			RequiredVersion: shape.minVersion,
		})
	}
	return events
}

// detectInexpressibleShape is API-072. Its findings carry no Suggestion on
// purpose: there is no spelling at this pin to suggest.
func detectInexpressibleShape(in fileInput) []Finding {
	var findings []Finding
	for _, gap := range inexpressibleShapes(in.file, in.desiredVersion) {
		findings = append(findings, Finding{
			RuleID:  "API-072",
			File:    in.filename,
			Line:    1,
			Message: gap.Shape + " is the canonical shape here, but pin " + gap.DesiredVersion + " cannot express it (needs " + gap.RequiredVersion + "); a product gap was recorded",
		})
	}
	return findings
}

// GoSourceWithGapSink reviews src like GoSourceAt and emits each product
// gap it finds to sink once per sink and signature.
func GoSourceWithGapSink(filename, src, desiredVersion string, sink GapSink) Result {
	res := GoSourceAt(filename, src, desiredVersion)
	_, f, err := parseSourceFile(filename, src)
	if err != nil || sink == nil {
		return res
	}
	for _, gap := range inexpressibleShapes(f, desiredVersion) {
		if gapLedger.firstEmission(sink, gap.Signature) {
			sink.Emit(gap)
		}
	}
	return res
}

// gapLedger remembers which signatures each sink has already received. A
// sink whose dynamic type is not comparable cannot be keyed and receives
// every event.
var gapLedger = &emissionLedger{seen: map[emissionKey]bool{}}

type emissionKey struct {
	sink      GapSink
	signature string
}

type emissionLedger struct {
	mu   sync.Mutex
	seen map[emissionKey]bool
}

// firstEmission reports whether signature has not yet gone to sink, and
// records that it now has.
func (l *emissionLedger) firstEmission(sink GapSink, signature string) bool {
	if !reflect.TypeOf(sink).Comparable() {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key := emissionKey{sink, signature}
	if l.seen[key] {
		return false
	}
	l.seen[key] = true
	return true
}
