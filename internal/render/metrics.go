package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// metricsLabel names the Verbose-only §39 timing line, rendered in the
// same dim "name  value" shape as a run Fact.
const metricsLabel = "timing"

// metricsSeparator joins the timing line's clauses.
const metricsSeparator = " · "

// nestedSeparator joins the spans a clause contains, inside its
// parentheses.
const nestedSeparator = ", "

// spanDisplayResolution is the smallest span formatSpan can show; a shorter
// span would render as a misleading "0ms" and is omitted instead.
const spanDisplayResolution = time.Millisecond

// secondsSpanPrecision is what a span between one second and one minute
// rounds to: tenths of a second ("1.2s").
const secondsSpanPrecision = 100 * time.Millisecond

// WriteMetrics renders the run's derived §39 aggregate as one dim line:
// how work and tracked operations resolved, where time went (running,
// waiting on dependencies, waiting on scheduler capacity, inside
// definitions, evaluating Verify), the critical path, and peak
// concurrency. Checking provenance and verifying tracked state happen
// inside Define callbacks, so they print in parentheses after the
// definitions span they are part of, never as sibling buckets. Zero clauses are omitted; a run with nothing
// to say writes nothing. The caller gates it on Verbose: rows are scarce
// (contract §13).
func WriteMetrics(b *strings.Builder, m core.RunMetrics, color bool) {
	clauses := append(resolutionClauses(m), timeClauses(m)...)
	if len(clauses) == 0 {
		return
	}
	line := core.Fact{Name: metricsLabel, Value: strings.Join(clauses, metricsSeparator)}
	fmt.Fprintf(b, "%s\n", txt.Dim(factText(line), color))
}

// clauseList collects the clauses whose value is worth saying.
type clauseList []string

func (c *clauseList) add(ok bool, clause string) {
	if ok {
		*c = append(*c, clause)
	}
}

func (c *clauseList) addSpan(d time.Duration, label string) {
	c.add(showsSpan(d), spanClause(d, label))
}

// showsSpan reports whether d is long enough to show at display precision.
func showsSpan(d time.Duration) bool { return d >= spanDisplayResolution }

func spanClause(d time.Duration, label string) string { return formatSpan(d) + " " + label }

func resolutionClauses(m core.RunMetrics) clauseList {
	var c clauseList
	ops := m.Operations
	c.add(m.Executed > 0, fmt.Sprintf("%d executed", m.Executed))
	c.add(m.AlreadySatisfied > 0, fmt.Sprintf("%d already satisfied", m.AlreadySatisfied))
	c.add(m.NoWork > 0, fmt.Sprintf("%d no work", m.NoWork))
	c.add(ops.Current > 0, fmt.Sprintf("%d of %d operations current", ops.Current, ops.Current+ops.Executed))
	c.add(ops.BasisDrift > 0, fmt.Sprintf("%d basis changed", ops.BasisDrift))
	c.add(ops.Unchanged > 0, fmt.Sprintf("%d identical outputs", ops.Unchanged))
	return c
}

func timeClauses(m core.RunMetrics) clauseList {
	var c clauseList
	c.addSpan(m.Running, "running")
	c.addSpan(m.DependencyWait, "waiting on dependencies")
	c.addSpan(m.SchedulerWait, "waiting on capacity")
	c.add(showsSpan(m.Definition), definitionClause(m))
	c.addSpan(m.Evidence, "evaluating Verify")
	c.addSpan(m.CriticalPath, "critical path")
	c.add(m.PeakConcurrency > 0, fmt.Sprintf("peak %d concurrent", m.PeakConcurrency))
	return c
}

// definitionClause renders time inside Define callbacks, followed by the
// provenance and tracked-state spans nested inside it.
func definitionClause(m core.RunMetrics) string {
	var nested clauseList
	nested.addSpan(m.Provenance, "checking provenance")
	nested.addSpan(m.TrackedState, "verifying tracked state")
	clause := spanClause(m.Definition, "in definitions")
	if len(nested) == 0 {
		return clause
	}
	return clause + " (" + strings.Join(nested, nestedSeparator) + ")"
}

// formatSpan renders a metric duration at a precision that still means
// something at its scale: whole milliseconds under a second, tenths of a
// second under a minute ("1.2s", "3s"), Go's rounded Duration past that.
func formatSpan(d time.Duration) string {
	switch {
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Minute:
		tenths := d.Round(secondsSpanPrecision).Seconds()
		return strconv.FormatFloat(tenths, 'f', -1, 64) + "s"
	default:
		return d.Round(time.Second).String()
	}
}
