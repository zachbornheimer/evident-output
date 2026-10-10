// Package mcp_test holds contract §21 MCP detectors that are not built yet.
// Each test fails today and turns green when review can detect the pattern.
package mcp_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

const (
	wireSchemaRuleID = "EVO-WIRE-002"
	glyphRuleID      = "EVO-UI-004"

	// encoderEditWithoutVersionBump adds a field to the wire document and
	// leaves the schema version constant as it was.
	encoderEditWithoutVersionBump = `package render

const JSONSchemaVersion = "0.4"

type JSONDocument struct {
	SchemaVersion string ` + "`json:\"schema_version\"`" + `
	Tasks         []string ` + "`json:\"tasks\"`" + `
	ExtraField    string ` + "`json:\"extra_field\"`" + `
}
`

	// callerChosenStatus hand-picks a color and a status word for a result.
	callerChosenStatus = `package main

import "fmt"

func report(name string) {
	fmt.Print("\x1b[32m[OK]\x1b[0m ", name, "\n")
}
`
)

func requireFinding(t *testing.T, result review.Result, ruleID, what string) {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.RuleID == ruleID {
			return
		}
	}
	t.Fatalf("review reported no %s finding for %s: %+v", ruleID, what, result.Findings)
}

func TestC21_014_MCPDetectsABreakingWireSchemaEdit(t *testing.T) {
	rule, ok := rules.Explain(wireSchemaRuleID)
	if !ok {
		t.Fatalf("rule %s is not in the catalog", wireSchemaRuleID)
	}
	if rule.Detection == rules.DetectionGuidance {
		t.Fatalf("%s is guidance-only: review has no detector for a wire-schema edit that keeps the same schema_version", wireSchemaRuleID)
	}
	result := review.GoSource("internal/render/json.go", encoderEditWithoutVersionBump)
	requireFinding(t, result, wireSchemaRuleID, "an encoder field added without a schema_version bump")
}

func TestC21_012_MCPDetectsACallerChosenGlyphColorOrStatus(t *testing.T) {
	result := review.GoSource("report.go", callerChosenStatus)
	requireFinding(t, result, glyphRuleID, "a hand-picked color and [OK] status")
}
