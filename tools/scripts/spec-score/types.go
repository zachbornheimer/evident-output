package main

import "strings"

// Tier is the release line a requirement belongs to.
type Tier string

const (
	Tier11 Tier = "1.1"
	Tier12 Tier = "1.2"
)

// Status is the outcome of scoring one requirement.
type Status string

const (
	StatusPass   Status = "pass"
	StatusFail   Status = "fail"
	StatusWaived Status = "waived"
)

// TestRef names one Go test bound to a requirement.
type TestRef struct {
	Pkg string `json:"pkg"`
	Run string `json:"run"`
}

// Entry is one requirement in the registry.
type Entry struct {
	ID      string    `json:"id"`
	Section string    `json:"section"`
	Quote   string    `json:"quote"`
	Tier    Tier      `json:"tier"`
	Tests   []TestRef `json:"tests"`
	Waiver  *string   `json:"waiver,omitempty"`
	Default bool      `json:"default"`
}

// WaiverReason is the reason the entry's waiver gives, trimmed; empty when
// the entry has no waiver or its waiver gives no reason.
func (e Entry) WaiverReason() string {
	if e.Waiver == nil {
		return ""
	}
	return strings.TrimSpace(*e.Waiver)
}

// Waived reports whether the entry carries a waiver with a reason, so no test
// is run for it and it scores as waived.
func (e Entry) Waived() bool { return e.WaiverReason() != "" }

// TierTotals counts scoring outcomes for one tier.
type TierTotals struct {
	Pass   int `json:"pass"`
	Fail   int `json:"fail"`
	Waived int `json:"waived"`
	Total  int `json:"total"`
}

// SectionTotals counts passing requirements in one contract section.
type SectionTotals struct {
	Pass  int `json:"pass"`
	Total int `json:"total"`
}

// Score is the machine-readable result of one scoring run.
type Score struct {
	IDs      map[string]Status        `json:"ids"`
	Sections map[string]SectionTotals `json:"sections"`
	Tiers    map[Tier]TierTotals      `json:"tiers"`
}
