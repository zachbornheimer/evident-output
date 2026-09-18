package rules

// stampRules is the EVO-STAMP-*/EVO-FACT-001/EVO-EFFECT-001 family: Done
// used as a print, a duplicate sibling label, a fake-success Fact, or a
// planned mutation narrated as if it already succeeded.
func stampRules() []Rule {
	return []Rule{
		{
			ID:        "EVO-STAMP-001",
			Category:  "STAMP",
			Severity:  "warning",
			Invariant: "Task.Done resolves already-submitted work; it is not a generic printf",
			Why:       "Task(...).Done(\"processed %s\", path) with no preceding Define/File uses the success stamp as a print statement. The row claims work finished that evo never scheduled, and the format string is unstructured text no renderer/JSON consumer can project.",
			BadCode: `out.Task("house-style").Done("processed %s", path)
`,
			GoodCode: `t := out.Task("write " + path)
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
`,
			Remediation:     "Submit the work with Define or evo.File, then Done() with no format-string prose — or delete the stamp if Define already resolves the task",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-STAMP-002",
			Category:  "STAMP",
			Severity:  "error",
			Invariant: "each sibling Task under one parent has a distinct name",
			Why:       "Repeating Task(\"house-style\") in a loop (or twice in one function) is a duplicate sibling declaration: 1.0 creates a second Failed row and records ErrDuplicateSiblingName instead of merging, because a merged identity could later report a false already-satisfied.",
			BadCode: `for _, path := range paths {
  out.Task("house-style").Done("ok")
}
`,
			GoodCode: `files := evo.Group("house-style")
for _, path := range paths {
  files.Task(path).Define(func() error { return write(path) })
}
`,
			Remediation:     "Give each sibling a distinct name — one Group.Task per item (the loop variable), never the same literal every iteration",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-FACT-001",
			Category:  "FACT",
			Severity:  "warning",
			Invariant: "discovered information is a Fact, never a fake Task success",
			Why:       "Done(\"mapped to /tmp/out\") paints a checkmark for information, not work. Facts are name/value observations the renderer already projects; a success stamp for them lies about a job that never ran.",
			BadCode:   `out.Task("mapping").Done("mapped to %s", dest)`,
			GoodCode: `t := out.Task("scan")
t.Fact("mapped to", dest)
`,
			Remediation:     "Replace Done(\"mapped to...\") with task.Fact(name, value) on the Task that discovered the information",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-FACT-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-EFFECT-001",
			Category:  "EFFECT",
			Severity:  "warning",
			Invariant: "a planned mutation is a DryRun mutation verb or Record, never Done(\"would add\")",
			Why:       "Done(\"would add %s\", path) or Done(\"proposal only\") stamps the task successful for work that has not happened. Config.DryRun already picks [planned] vs [changed] on Create/Delete/Update; Record tallies the effect. Narrating the plan through Done makes a dry-run look like a completed success.",
			BadCode: `out.Task("proposal").Done("would add %s", path)
out.Task("plan").Done("proposal only")
`,
			GoodCode: `out := evo.Init(evo.Config{DryRun: true})
t := out.Task("house-style")
t.Create("config", func() error { return write(path) })
// or, for a tally with no callback: t.Record("add", 1, "config")
`,
			Remediation:     "Use a mutation verb under Config.DryRun, or Record, instead of Done(\"would ...\" / \"proposal only\")",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-EFFECT-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(stampRules()) }
