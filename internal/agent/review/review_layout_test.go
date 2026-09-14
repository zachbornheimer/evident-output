package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const (
	layoutDualNaming  = "LAYOUT-001"
	layoutWrongFolder = "LAYOUT-002"
)

const layout001DirtyFile = `package app

import "github.com/spf13/cobra"

func Command() *cobra.Command {
	return &cobra.Command{
		Use: "prune",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
`

const layout001DirtyIdent = `package app

import "github.com/spf13/cobra"

const cleanRepoCommandName = "prune"

func Command() *cobra.Command {
	return &cobra.Command{
		Use: cleanRepoCommandName,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
`

const layout001Clean = `package prune

import "github.com/spf13/cobra"

func Command() *cobra.Command {
	return &cobra.Command{
		Use:     "prune",
		Aliases: []string{"clean-repo"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
`

const layout002Dirty = `package app

import "github.com/spf13/cobra"

func Command() *cobra.Command {
	return &cobra.Command{
		Use: "purge",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := walkAndPurge(); err != nil {
				return err
			}
			return nil
		},
	}
}

func walkAndPurge() error { return nil }
`

const layout002CleanOwner = `package purge

import "github.com/spf13/cobra"

func Command() *cobra.Command {
	return &cobra.Command{
		Use: "purge",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := walkAndPurge(); err != nil {
				return err
			}
			return nil
		},
	}
}

func walkAndPurge() error { return nil }
`

const layout002CleanDelegate = `package app

import "example.com/tool/internal/purge"

func purgeCommand() *cobra.Command {
	return purge.Command()
}
`

func TestLAYOUT001_CleanRepoFilenameWithUsePruneIsDirty(t *testing.T) {
	res := review.GoSource("clean_repo.go", layout001DirtyFile)
	if !hasRuleID(res, layoutDualNaming) {
		t.Fatalf("expected LAYOUT-001 on clean_repo.go registering Use prune: %+v", res.Findings)
	}
}

func TestLAYOUT001_CleanRepoIdentAssignedPruneIsDirty(t *testing.T) {
	res := review.GoSource("cmd.go", layout001DirtyIdent)
	if !hasRuleID(res, layoutDualNaming) {
		t.Fatalf("expected LAYOUT-001 on cleanRepoCommandName = \"prune\": %+v", res.Findings)
	}
}

func TestLAYOUT001_PruneGoWithCleanRepoAliasIsClean(t *testing.T) {
	res := review.GoSource("prune.go", layout001Clean)
	if hasRuleID(res, layoutDualNaming) {
		t.Fatalf("prune.go Use prune + Aliases clean-repo must not be LAYOUT-001: %+v", res.Findings)
	}
}

func TestLAYOUT002_PurgeRunEInInternalAppIsDirty(t *testing.T) {
	res := review.GoSource("internal/app/purge.go", layout002Dirty)
	if !hasRuleID(res, layoutWrongFolder) {
		t.Fatalf("expected LAYOUT-002 on purge RunE under internal/app/: %+v", res.Findings)
	}
}

func TestLAYOUT002_PurgeRunEInInternalPurgeIsClean(t *testing.T) {
	res := review.GoSource("internal/purge/purge.go", layout002CleanOwner)
	if hasRuleID(res, layoutWrongFolder) {
		t.Fatalf("internal/purge owning RunE must not be LAYOUT-002: %+v", res.Findings)
	}
}

func TestLAYOUT002_ThinAppDelegateToPurgeCommandIsClean(t *testing.T) {
	res := review.GoSource("internal/app/app.go", layout002CleanDelegate)
	if hasRuleID(res, layoutWrongFolder) {
		t.Fatalf("thin internal/app return purge.Command() must not be LAYOUT-002: %+v", res.Findings)
	}
}

func TestLAYOUT001And002_DoNotRequireEvoImport(t *testing.T) {
	res := review.GoSource("clean_repo.go", layout001DirtyFile)
	if !hasRuleID(res, layoutDualNaming) {
		t.Fatalf("LAYOUT-001 must fire without an evo import: %+v", res.Findings)
	}
	res = review.GoSource("internal/app/cmd.go", layout002Dirty)
	if !hasRuleID(res, layoutWrongFolder) {
		t.Fatalf("LAYOUT-002 must fire without an evo import: %+v", res.Findings)
	}
}

func hasRuleID(res review.Result, id string) bool {
	for _, f := range res.Findings {
		if f.RuleID == id {
			return true
		}
	}
	return false
}

func TestLAYOUT001_FindingNamesLeftover(t *testing.T) {
	res := review.GoSource("clean_repo.go", layout001DirtyFile)
	for _, f := range res.Findings {
		if f.RuleID != layoutDualNaming {
			continue
		}
		blob := f.Message + " " + f.Suggestion
		if f.Line == 0 {
			t.Error("LAYOUT-001 missing line")
		}
		if !strings.Contains(blob, "prune") && !strings.Contains(blob, "clean-repo") && !strings.Contains(blob, "clean_repo") {
			t.Fatalf("LAYOUT-001 must name the leftover dual naming, got message=%q suggestion=%q", f.Message, f.Suggestion)
		}
		return
	}
	t.Fatalf("expected LAYOUT-001: %+v", res.Findings)
}
