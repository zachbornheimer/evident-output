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

// WriteMetrics renders the run's derived §39 aggregate as one dim line:
// how work resolved, then where time went — running, waiting on
// dependencies, waiting on scheduler capacity — and peak concurrency.
// Zero clauses are omitted; a run with nothing to say writes nothing. The
// caller gates it on Verbose: rows are scarce (contract §13).
func WriteMetrics(b *strings.Builder, m core.RunMetrics, color bool) {
	clauses := metricsClauses(m)
	if len(clauses) == 0 {
		return
	}
	line := core.Fact{Name: metricsLabel, Value: strings.Join(clauses, metricsSeparator)}
	fmt.Fprintf(b, "%s\n", txt.Dim(factText(line), color))
}

func metricsClauses(m core.RunMetrics) []string {
	var clauses []string
	add := func(ok bool, clause string) {
		if ok {
			clauses = append(clauses, clause)
		}
	}
	add(m.Executed > 0, fmt.Sprintf("%d executed", m.Executed))
	add(m.AlreadySatisfied > 0, fmt.Sprintf("%d already satisfied", m.AlreadySatisfied))
	add(m.NoWork > 0, fmt.Sprintf("%d no work", m.NoWork))
	add(m.Running > 0, formatSpan(m.Running)+" running")
	add(m.DependencyWait > 0, formatSpan(m.DependencyWait)+" waiting on dependencies")
	add(m.SchedulerWait > 0, formatSpan(m.SchedulerWait)+" waiting on capacity")
	add(m.PeakConcurrency > 0, fmt.Sprintf("peak %d concurrent", m.PeakConcurrency))
	return clauses
}

// formatSpan renders a metric duration at a precision that still means
// something at its scale: whole milliseconds under a second, tenths of a
// second under a minute ("1.2s", "3s"), Go's rounded Duration past that.
func formatSpan(d time.Duration) string {
	switch {
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Minute:
		tenths := d.Round(100 * time.Millisecond).Seconds()
		return strconv.FormatFloat(tenths, 'f', -1, 64) + "s"
	default:
		return d.Round(time.Second).String()
	}
}
