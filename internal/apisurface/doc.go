// Package apisurface walks the root evo package's exported API and checks
// it against the committed golden, the spec-required identifier floor, and
// the retired-name ban. `evident-output contract` and `mise run api-contract`
// run that check.
package apisurface
