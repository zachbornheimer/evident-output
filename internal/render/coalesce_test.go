package render

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestShouldSuppressStandaloneConclusion_TwoChangesSections_SuppressesTitleBand(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{
		Subject: "zq",
		Changes: []core.ChangesSnapshot{
			{
				Subject: ".prettierrc.json",
				Records: []core.EffectRecord{{Verb: "wrote", Object: ".prettierrc.json"}},
			},
			{
				Subject: ".golangci.yml",
				Records: []core.EffectRecord{{Verb: "wrote", Object: ".golangci.yml"}},
			},
		},
		Conclusion: &core.Conclusion{State: core.StateChanged, Subject: "zq"},
	}
	if !ShouldSuppressStandaloneConclusion(snap) {
		t.Fatal("two Changes sections plus a title-only changed conclusion must suppress the trailing band")
	}
	got := Plain(snap, 0, true, false, txt.GlyphsASCII)
	if strings.Contains(got, "[changed]  zq") {
		t.Fatalf("human projection still printed the title band:\n%s", got)
	}
}

func TestShouldSuppressStandaloneConclusion_WarnedPlannedWithPlan_SuppressesTitleBand(t *testing.T) {
	t.Parallel()
	snap := core.Snapshot{
		Subject: "zq",
		Tasks: []core.TaskSnapshot{
			{
				Name:     "branches",
				State:    core.Done,
				Warnings: []core.Problem{{Summary: "kept 13"}},
			},
		},
		Plans: []core.PlanSnapshot{
			{
				Subject: "branches",
				Records: []core.EffectRecord{{Verb: "delete", Quantity: 2, HasQty: true, Object: "local tip"}},
			},
		},
		Conclusion: &core.Conclusion{State: core.StatePlanned, Subject: "zq", Warned: true},
	}
	if !ShouldSuppressStandaloneConclusion(snap) {
		t.Fatal("a warned planned run with a Plan ledger must suppress [planned · warned] zq")
	}
	got := Plain(snap, 0, true, false, txt.GlyphsASCII)
	if strings.Contains(got, "[planned · warned]") {
		t.Fatalf("human projection still printed the title band:\n%s", got)
	}
}
