package evo_test

// TestKept_ConcludesWarnedInHumanAndMachineOutput pinned the contract §18
// requirement that a Kept record feeds the conclusion's warned dimension,
// in both the human "· warned" band and the machine --json
// conclusion.warned field. Kept was retired in 1.1 (Skipped wins,
// §"Duplicate decisions" in the vocabulary freeze): the public evo package
// has no Kept method any more, so a Kept record can no longer be produced
// through the public API, and there is nothing left here to pin. The
// counterpart this file existed alongside — a Skipped record does NOT feed
// warned, in either the human band or the machine conclusion — is still
// live and covered by TestTaskHandle_SkippedTallyUsesSkipDetailGlyphNotWarning
// (taxonomy_test.go), which asserts both the absent "warned" band in the
// human transcript and out.Conclusion().Warned == false.
