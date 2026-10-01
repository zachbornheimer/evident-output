package driver_test

import (
	"errors"
	"math"
	"testing"

	"github.com/zachbornheimer/evident-output/eval/driver"
)

func TestPrice_CostCountsEveryTokenKind(t *testing.T) {
	price := driver.Price{InputPerMTok: 1, OutputPerMTok: 2, CacheReadPerMTok: 3, CacheWritePerMTok: 4}
	usage := driver.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000}
	if got := price.Cost(usage); math.Abs(got-10) > 1e-9 {
		t.Errorf("cost = %v, want 10", got)
	}
}

func TestPriceTable_RefusesUnverifiedPlaceholder(t *testing.T) {
	table := driver.PriceTable(driver.PlaceholderPrices)
	if _, err := table.Resolve(driver.ModelInnerLoop); err == nil {
		t.Fatal("placeholder price must not resolve without an override")
	}
}

func TestPriceTable_OverrideCountsAsVerified(t *testing.T) {
	overrides := map[string]driver.Price{driver.ModelInnerLoop: {InputPerMTok: 1}}
	price, err := driver.PriceTable(driver.PlaceholderPrices).WithOverrides(overrides).Resolve(driver.ModelInnerLoop)
	if err != nil || price.InputPerMTok != 1 {
		t.Fatalf("override not applied: %+v %v", price, err)
	}
}

func TestParsePriceOverride(t *testing.T) {
	model, price, err := driver.ParsePriceOverride("m=3,15,0.3,3.75")
	if err != nil || model != "m" || price.OutputPerMTok != 15 || price.CacheWritePerMTok != 3.75 {
		t.Fatalf("parse = %q %+v %v", model, price, err)
	}
	for _, bad := range []string{"m", "=1,2,3,4", "m=1,2,3", "m=a,2,3,4", "m=-1,2,3,4"} {
		if _, _, err := driver.ParsePriceOverride(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestSpendGuard_TripsAtCap(t *testing.T) {
	guard, err := driver.NewSpendGuard(1)
	if err != nil {
		t.Fatalf("NewSpendGuard: %v", err)
	}
	if err := guard.Add(0.6); err != nil {
		t.Fatalf("below cap: %v", err)
	}
	if err := guard.Add(0.4); !errors.Is(err, driver.ErrSpendCapReached) {
		t.Fatalf("at cap: got %v, want ErrSpendCapReached", err)
	}
	if err := guard.Check(); !errors.Is(err, driver.ErrSpendCapReached) {
		t.Fatalf("Check after cap: got %v", err)
	}
}

func TestNewSpendGuard_RejectsNonPositiveCap(t *testing.T) {
	if _, err := driver.NewSpendGuard(0); err == nil {
		t.Fatal("a zero cap must be rejected")
	}
}
