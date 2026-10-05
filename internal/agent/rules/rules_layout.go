package rules

// layoutRules is command naming and folder ownership (LAYOUT).
func layoutRules() []Rule {
	return []Rule{
		{
			ID:        "LAYOUT-001",
			Category:  "LAYOUT",
			Severity:  SeverityWarning,
			Invariant: "a cobra command's file and identifiers match Use; leftover clean-repo naming is not a second primary name",
			Why:       "A file named clean_repo.go, or an identifier such as cleanRepoCommandName = \"prune\", that registers cobra Use \"prune\" keeps two names for one command. Reviewers and agents cannot tell which is canonical.",
			BadCode: `// file: clean_repo.go
const cleanRepoCommandName = "prune"
cmd := &cobra.Command{Use: cleanRepoCommandName}`,
			GoodCode: `// file: prune.go (package prune)
cmd := &cobra.Command{
  Use: "prune",
  Aliases: []string{"clean-repo"},
}`,
			Remediation:     "Name the file and identifiers after cobra Use (prune/purge); keep clean-repo only as Aliases",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"LAYOUT-001"},
			Since:           "0.5.1",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "LAYOUT-002",
			Category:  "LAYOUT",
			Severity:  SeverityWarning,
			Invariant: "cobra Use purge/prune RunE lives in internal/<cmd>/, not internal/app/",
			Why:       "A real RunE body under internal/app/ hides the command behind a catch-all folder. The owning package is internal/purge or internal/prune; app may only return that Command().",
			BadCode: `// file: internal/app/purge.go
cmd := &cobra.Command{
  Use: "purge",
  RunE: func(cmd *cobra.Command, args []string) error {
    return walkAndPurge()
  },
}`,
			GoodCode: `// file: internal/purge/purge.go owns RunE
func Command() *cobra.Command {
  return &cobra.Command{Use: "purge", RunE: run}
}
// file: internal/app/app.go — thin delegate
func purgeCommand() *cobra.Command { return purge.Command() }`,
			Remediation:     "Move the RunE body into internal/purge or internal/prune; leave a one-return *.Command() delegate in internal/app",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"LAYOUT-002"},
			Since:           "0.5.1",
			Certainty:       CertaintyDeterministic,
		},
	}
}

func init() { registerFamily(layoutRules()) }
