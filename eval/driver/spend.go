package driver

import (
	"errors"
	"fmt"
)

// ErrSpendCapReached aborts a run: the accumulated cost reached --max-usd.
var ErrSpendCapReached = errors.New("spend cap reached")

// SpendGuard accumulates dollars and trips at a hard cap.
type SpendGuard struct {
	capUSD   float64
	spentUSD float64
}

// NewSpendGuard builds a guard; a cap that is not positive is a caller bug.
func NewSpendGuard(capUSD float64) (*SpendGuard, error) {
	if capUSD <= 0 {
		return nil, fmt.Errorf("build spend guard: cap must be positive, got %.4f", capUSD)
	}
	return &SpendGuard{capUSD: capUSD}, nil
}

// Add records cost and reports ErrSpendCapReached once the total reaches the
// cap, so the caller stops before the next request.
func (g *SpendGuard) Add(costUSD float64) error {
	g.spentUSD += costUSD
	return g.Check()
}

// Check reports ErrSpendCapReached when the cap has been reached.
func (g *SpendGuard) Check() error {
	if g.spentUSD >= g.capUSD {
		return fmt.Errorf("spent $%.4f of $%.4f: %w", g.spentUSD, g.capUSD, ErrSpendCapReached)
	}
	return nil
}

// Spent is the dollars recorded so far.
func (g *SpendGuard) Spent() float64 { return g.spentUSD }
