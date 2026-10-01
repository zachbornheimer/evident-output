package driver

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
)

// Model ids the eval runs against.
const (
	ModelInnerLoop    = "claude-sonnet-5-5"
	ModelConfirmation = "claude-opus-5-5"
)

const tokensPerMTok = 1_000_000.0

// Price is USD per million tokens. Verified is false until someone has
// checked the figures against the Anthropic pricing page.
type Price struct {
	InputPerMTok      float64 `json:"input_per_mtok"`
	OutputPerMTok     float64 `json:"output_per_mtok"`
	CacheReadPerMTok  float64 `json:"cache_read_per_mtok"`
	CacheWritePerMTok float64 `json:"cache_write_per_mtok"`
	Verified          bool    `json:"verified"`
}

// PlaceholderPrices holds UNVERIFIED figures. They were NOT checked against
// the Anthropic pricing page (the Go SDK carries model ids but no prices, and
// no network was available when this table was written), so the runner
// refuses to start on any entry with Verified=false unless the caller passes
// --price-override. To verify: read the pricing page, edit the numbers, and
// flip Verified to true.
var PlaceholderPrices = map[string]Price{
	ModelInnerLoop:    {InputPerMTok: 3, OutputPerMTok: 15, CacheReadPerMTok: 0.30, CacheWritePerMTok: 3.75},
	ModelConfirmation: {InputPerMTok: 5, OutputPerMTok: 25, CacheReadPerMTok: 0.50, CacheWritePerMTok: 6.25},
}

// Cost is the dollars one Usage bills at this price.
func (p Price) Cost(u Usage) float64 {
	return (float64(u.InputTokens)*p.InputPerMTok +
		float64(u.OutputTokens)*p.OutputPerMTok +
		float64(u.CacheReadTokens)*p.CacheReadPerMTok +
		float64(u.CacheWriteTokens)*p.CacheWritePerMTok) / tokensPerMTok
}

// PriceTable resolves the price of a model.
type PriceTable map[string]Price

// Resolve returns the price for model, or an error naming why it cannot be
// trusted: unknown model, or an unverified entry with no override.
func (t PriceTable) Resolve(model string) (Price, error) {
	price, ok := t[model]
	if !ok {
		return Price{}, fmt.Errorf("resolve price for model %q: not in the price table", model)
	}
	if !price.Verified {
		return Price{}, fmt.Errorf("resolve price for model %q: the table entry is an unverified placeholder; verify it in eval/driver/pricing.go or pass --price-override %s=<input>,<output>,<cache-read>,<cache-write> (USD per million tokens)", model, model)
	}
	return price, nil
}

// WithOverrides returns the table with each override applied as a verified
// price: the caller vouched for it on the command line.
func (t PriceTable) WithOverrides(overrides map[string]Price) PriceTable {
	merged := PriceTable{}
	maps.Copy(merged, t)
	for model, price := range overrides {
		price.Verified = true
		merged[model] = price
	}
	return merged
}

const overrideFields = 4

// ParsePriceOverride parses "model=in,out,cacheRead,cacheWrite".
func ParsePriceOverride(spec string) (string, Price, error) {
	model, figures, ok := strings.Cut(spec, "=")
	if !ok || model == "" {
		return "", Price{}, fmt.Errorf("parse price override %q: want model=in,out,cache-read,cache-write", spec)
	}
	parts := strings.Split(figures, ",")
	if len(parts) != overrideFields {
		return "", Price{}, fmt.Errorf("parse price override %q: want %d comma-separated figures, got %d", spec, overrideFields, len(parts))
	}
	var values [overrideFields]float64
	for i, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || value < 0 {
			return "", Price{}, fmt.Errorf("parse price override %q: figure %q is not a non-negative number", spec, part)
		}
		values[i] = value
	}
	return model, Price{InputPerMTok: values[0], OutputPerMTok: values[1], CacheReadPerMTok: values[2], CacheWritePerMTok: values[3]}, nil
}
