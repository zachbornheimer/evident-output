# Architecture completeness matrix (spec v0.3 §31)

Source of truth: `conformance/TRACEABILITY.md` + automated tests.

| Metric | Value |
|--------|-------|
| Total §31 IDs | 272 |
| pass | 267 |
| waived (external/manual) | 5 |
| untested | 0 |

## Family rollup

| Family | pass | waived | total |
|--------|------|--------|-------|
| A11Y | 8 | 2 | 10 |
| API | 30 | 0 | 30 |
| CON | 19 | 0 | 19 |
| DOM | 50 | 0 | 50 |
| LOG | 15 | 0 | 15 |
| MCP | 50 | 0 | 50 |
| OUT | 24 | 0 | 24 |
| PORT | 12 | 3 | 15 |
| SEC | 15 | 0 | 15 |
| TERM | 24 | 0 | 24 |
| TXT | 20 | 0 | 20 |

## Non-pass items (only remaining gaps)

- **A11Y-006** (waived): light/dark theme manual contrast review — owner `zachbornheimer`
- **A11Y-007** (waived): screen-reader manual review — owner `zachbornheimer`
- **PORT-003** (waived): Windows ConPTY — release-candidate manual checklist — owner `zachbornheimer`
- **PORT-004** (waived): tmux — manual RC — owner `zachbornheimer`
- **PORT-005** (waived): SSH — manual RC — owner `zachbornheimer`

These are host/manual release-candidate items. Spec §31 allows waiver with reason + owner.

## Skeptic-flagged agent rows (closed on main)

| ID | Evidence |
|----|----------|
| MCP-014 | `agent/review TestMCP014_* + harness blocked-as-error` |
| MCP-022 | `agent/harness TestMCP022_RepairLoopReachesClean + RunAllRepairable` |
| MCP-027 | `agent/rules TestMCP027_ExplainFullPayload` |
| MCP-028 | `agent/rules TestMCP028_RuleStabilityVersionPolicy` |
| MCP-034 | `mcp_hardening_test.go TestMCP034_PanicContainedContinues (faultHook panic injection)` |
| MCP-049 | `agent/harness TestMCP049_StopOnlyWhenRecheckFalse` |

## Full inventory

| ID | Status | Evidence |
|----|--------|----------|
| A11Y-001 | pass | coverage_matrix_test.go |
| A11Y-002 | pass | coverage_matrix_test.go |
| A11Y-003 | pass | capability NoColor |
| A11Y-004 | pass | coverage_matrix_test.go |
| A11Y-005 | pass | coverage_matrix_test.go |
| A11Y-006 | waived | light/dark theme manual contrast review |
| A11Y-007 | waived | screen-reader manual review |
| A11Y-008 | pass | no blink static |
| A11Y-009 | pass | final_matrix_test.go |
| A11Y-010 | pass | closeout_test.go |
| API-001 | pass | dom_test.go |
| API-002 | pass | appendix_h_test.go |
| API-003 | pass | appendix_h_test.go |
| API-004 | pass | last_push_test.go |
| API-005 | pass | last_push_test.go |
| API-006 | pass | agent/review |
| API-007 | pass | config.go NewWithConfig |
| API-008 | pass | last_push_test.go |
| API-009 | pass | remaining_core_test.go |
| API-010 | pass | remaining_core_test.go |
| API-011 | pass | closeout_test.go |
| API-012 | pass | last_push_test.go |
| API-013 | pass | examples/framework-adapters + release_matrix_test.go TestAPI013 |
| API-014 | pass | slog_test.go |
| API-015 | pass | debug_writer |
| API-016 | pass | projection_test.go |
| API-017 | pass | final_matrix_test.go |
| API-018 | pass | coverage_matrix_test.go |
| API-019 | pass | no global state race tests |
| API-020 | pass | closeout_test.go |
| API-021 | pass | examples compile |
| API-022 | pass | closeout_test.go |
| API-023 | pass | Line one call |
| API-024 | pass | closeout_test.go |
| API-025 | pass | last_push_test.go |
| API-026 | pass | more_matrix_test.go |
| API-027 | pass | appendix_h_test.go |
| API-028 | pass | last_push_test.go |
| API-029 | pass | last_push_test.go |
| API-030 | pass | last_push_test.go |
| CON-001 | pass | con_race_test.go |
| CON-002 | pass | final_matrix_test.go |
| CON-003 | pass | waived_closeout_test.go TestCON003_* |
| CON-004 | pass | waived_closeout_test.go TestCON004_ResizeWhileLive |
| CON-005 | pass | remaining_core_test.go |
| CON-006 | pass | closeout_test.go |
| CON-007 | pass | closeout_test.go |
| CON-008 | pass | waived_closeout_test.go TestCON008_JournalBackpressure |
| CON-009 | pass | waived_closeout_test.go TestCON009_MultiRendererOneFailure |
| CON-010 | pass | last_push_test.go |
| CON-011 | pass | final_matrix_test.go |
| CON-012 | pass | con_race_test.go |
| CON-013 | pass | final_matrix_test.go |
| CON-014 | pass | appendix_h_interactive_test.go H.22 |
| CON-015 | pass | closeout_test.go |
| CON-016 | pass | last_push_test.go |
| CON-017 | pass | closeout_test.go |
| CON-018 | pass | last_push_test.go |
| CON-019 | pass | closeout_test.go |
| DOM-001 | pass | appendix_h_test.go,dom_test.go |
| DOM-002 | pass | appendix_h_test.go |
| DOM-003 | pass | appendix_h_test.go |
| DOM-004 | pass | coverage_matrix_test.go |
| DOM-005 | pass | sec_limits_test.go |
| DOM-006 | pass | dom_test.go |
| DOM-007 | pass | appendix_h_test.go |
| DOM-008 | pass | appendix_h_test.go |
| DOM-009 | pass | appendix_h_test.go |
| DOM-010 | pass | dom_test.go |
| DOM-011 | pass | appendix_h_test.go |
| DOM-012 | pass | dom_test.go |
| DOM-013 | pass | coverage_matrix_test.go |
| DOM-014 | pass | more_matrix_test.go |
| DOM-015 | pass | more_matrix_test.go |
| DOM-016 | pass | appendix_h_test.go + live |
| DOM-017 | pass | appendix_h_test.go |
| DOM-018 | pass | appendix_h_test.go |
| DOM-019 | pass | dom_test.go |
| DOM-020 | pass | appendix_h_test.go |
| DOM-021 | pass | coverage_matrix_test.go |
| DOM-022 | pass | appendix_h_test.go |
| DOM-023 | pass | more_matrix_test.go |
| DOM-024 | pass | sec_limits_test.go |
| DOM-025 | pass | appendix_h_interactive_test.go H.2 |
| DOM-026 | pass | appendix_h_interactive_test.go H.17 |
| DOM-027 | pass | Donef H.17 |
| DOM-028 | pass | appendix_h_test.go |
| DOM-029 | pass | appendix_h_test.go |
| DOM-030 | pass | H.11 warning path |
| DOM-031 | pass | H.11 all done |
| DOM-032 | pass | appendix_h_test.go |
| DOM-033 | pass | dom_test.go |
| DOM-034 | pass | appendix_h_test.go |
| DOM-035 | pass | remaining_core_test.go |
| DOM-036 | pass | dom_test.go |
| DOM-037 | pass | more_matrix_test.go |
| DOM-038 | pass | more_matrix_test.go |
| DOM-039 | pass | dom_test.go |
| DOM-040 | pass | appendix_h_test.go |
| DOM-041 | pass | more_matrix_test.go |
| DOM-042 | pass | more_matrix_test.go |
| DOM-043 | pass | dom_test.go |
| DOM-044 | pass | dom_test.go |
| DOM-045 | pass | dom_test.go |
| DOM-046 | pass | dom_test.go |
| DOM-047 | pass | H.10 order |
| DOM-048 | pass | errgroup nil pattern docs |
| DOM-049 | pass | Output.Fail |
| DOM-050 | pass | common/advanced ItemWith |
| LOG-001 | pass | appendix_h_interactive_test.go H.17 |
| LOG-002 | pass | more_matrix_test.go |
| LOG-003 | pass | last_push_test.go |
| LOG-004 | pass | sensitive field |
| LOG-005 | pass | debug_writer_test.go |
| LOG-006 | pass | debug_writer_test.go |
| LOG-007 | pass | debug_writer.go max line |
| LOG-008 | pass | remaining_core_test.go |
| LOG-009 | pass | slog_test.go |
| LOG-010 | pass | final_matrix_test.go |
| LOG-011 | pass | closeout_test.go |
| LOG-012 | pass | final_matrix_test.go |
| LOG-013 | pass | closeout_test.go |
| LOG-014 | pass | WarnMessage vs item warn |
| LOG-015 | pass | last_push_test.go |
| MCP-001 | pass | mcp_lifecycle_test.go, cmd/evident-output-mcp/mcp_test.go |
| MCP-002 | pass | mcp_test.go |
| MCP-003 | pass | mcp_test.go |
| MCP-004 | pass | mcp_test.go |
| MCP-005 | pass | mcp_test.go |
| MCP-006 | pass | agent/catalog |
| MCP-007 | pass | agent/catalog |
| MCP-008 | pass | agent/catalog |
| MCP-009 | pass | mcp get_guidance missing |
| MCP-010 | pass | catalog.Checksum + mcp_hardening_test.go |
| MCP-011 | pass | agent/review |
| MCP-012 | pass | agent/review |
| MCP-013 | pass | mcp review stream |
| MCP-014 | pass | agent/review TestMCP014_* + harness blocked-as-error |
| MCP-015 | pass | review location |
| MCP-016 | pass | waived_closeout_test.go TestMCP016_PartialTypeinfoMarked |
| MCP-017 | pass | agent/review.GoPackage + review_test.go TestGoPackage_CrossFileTypes |
| MCP-018 | pass | agent/review.Transcript + review_test.go |
| MCP-019 | pass | agent/review.StructuredDocument + review_test.go |
| MCP-020 | pass | harness recheck |
| MCP-021 | pass | harness recheck |
| MCP-022 | pass | agent/harness TestMCP022_RepairLoopReachesClean + RunAllRepairable |
| MCP-023 | pass | agent/preview |
| MCP-024 | pass | preview profiles |
| MCP-025 | pass | waived_closeout_test.go TestMCP025_PreviewDebugInterleave |
| MCP-026 | pass | preview no exec |
| MCP-027 | pass | agent/rules TestMCP027_ExplainFullPayload |
| MCP-028 | pass | agent/rules TestMCP028_RuleStabilityVersionPolicy |
| MCP-029 | pass | mcp_hardening_test.go TestMCP029_030_StructuredAndText |
| MCP-030 | pass | mcp_hardening_test.go TestMCP029_030_StructuredAndText |
| MCP-031 | pass | oversized not crashed |
| MCP-032 | pass | mcp_hardening_test.go TestMCP032_DeadlineRespected |
| MCP-033 | pass | mcp_sec_test.go |
| MCP-034 | pass | mcp_hardening_test.go TestMCP034_PanicContainedContinues (faultHook panic injection) |
| MCP-035 | pass | mcp_sec_test.go |
| MCP-036 | pass | cmd/evident-output-mcp isRemotePath + mcp_hardening_test.go TestMCP036 |
| MCP-037 | pass | waived_closeout_test.go TestMCP037_ReviewDoesNotMutateSource |
| MCP-038 | pass | mcp_sec_test.go |
| MCP-039 | pass | mcp_sec_test.go |
| MCP-040 | pass | mcp resources |
| MCP-041 | pass | mcp_hardening_test.go TestMCP041_* |
| MCP-042 | pass | mcp_hardening_test.go TestMCP042_ToolNamesValid |
| MCP-043 | pass | mcp_hardening_test.go TestMCP043_UnknownFieldsRejected |
| MCP-044 | pass | stderr debug |
| MCP-045 | pass | mcp no http |
| MCP-046 | pass | cmd/evident-output/cli_test.go |
| MCP-047 | pass | skills skill |
| MCP-048 | pass | harness common-api |
| MCP-049 | pass | agent/harness TestMCP049_StopOnlyWhenRecheckFalse |
| MCP-050 | pass | catalog.ApplyTokenBudget + mcp_hardening_test.go |
| OUT-001 | pass | DataProjection |
| OUT-002 | pass | closeout_test.go |
| OUT-003 | pass | DataProjection |
| OUT-004 | pass | appendix_h H.18 |
| OUT-005 | pass | appendix_h_test.go |
| OUT-006 | pass | coverage_matrix_test.go |
| OUT-007 | pass | deterministic JSON |
| OUT-008 | pass | final_matrix_test.go |
| OUT-009 | pass | final_matrix_test.go |
| OUT-010 | pass | closeout_test.go |
| OUT-011 | pass | remaining_core_test.go |
| OUT-012 | pass | more_matrix_test.go |
| OUT-013 | pass | closeout_test.go |
| OUT-014 | pass | final_matrix_test.go |
| OUT-015 | pass | closeout_test.go |
| OUT-016 | pass | closeout_test.go |
| OUT-017 | pass | last_push_test.go |
| OUT-018 | pass | appendix_h_test.go RenderPlain/EncodeJSON |
| OUT-019 | pass | closeout_test.go |
| OUT-020 | pass | final_matrix_test.go |
| OUT-021 | pass | projection_test.go |
| OUT-022 | pass | final_matrix_test.go |
| OUT-023 | pass | last_push_test.go |
| OUT-024 | pass | last_push_test.go |
| PORT-001 | pass | port_pty_unix_test.go |
| PORT-002 | pass | port_pty_unix_test.go |
| PORT-003 | waived | Windows ConPTY — release-candidate manual checklist |
| PORT-004 | waived | tmux — manual RC |
| PORT-005 | waived | SSH — manual RC |
| PORT-006 | pass | NonInteractive plain |
| PORT-007 | pass | NO_COLOR option |
| PORT-008 | pass | width 0 fallback |
| PORT-009 | pass | capability height |
| PORT-010 | pass | last_push_test.go |
| PORT-011 | pass | waived_closeout_test.go TestPORT011_Int64ProgressPaths |
| PORT-012 | pass | release_matrix_test.go TestPORT012_BigEndianCrossCompile (GOARCH=s390x) |
| PORT-013 | pass | closeout_test.go |
| PORT-014 | pass | closeout_test.go |
| PORT-015 | pass | last_push_test.go |
| SEC-001 | pass | sanitize_test.go |
| SEC-002 | pass | sec_limits_test.go |
| SEC-003 | pass | more_matrix_test.go |
| SEC-004 | pass | closeout_test.go |
| SEC-005 | pass | sec_limits_test.go |
| SEC-006 | pass | coverage_matrix_test.go |
| SEC-007 | pass | sec_limits_test.go |
| SEC-008 | pass | mise scan + release_matrix_test.go TestSEC008_GovulncheckWhenInstalled |
| SEC-009 | pass | LICENSE Apache-2.0 + release_matrix_test.go TestSEC009 |
| SEC-010 | pass | closeout_test.go |
| SEC-011 | pass | sec_limits_test.go |
| SEC-012 | pass | last_push_test.go |
| SEC-013 | pass | final_matrix_test.go |
| SEC-014 | pass | mcp_sec_test.go + waived_closeout_test.go TestSEC014 |
| SEC-015 | pass | waived_closeout_test.go TestSEC015_NoAuthOnAnnotations |
| TERM-001 | pass | appendix_h_interactive_test.go H.2 |
| TERM-002 | pass | appendix_h_interactive_test.go H.2/H.17 |
| TERM-003 | pass | appendix_h_interactive_test.go H.17 |
| TERM-004 | pass | terminal/ansi_test.go |
| TERM-005 | pass | appendix_h + live |
| TERM-006 | pass | appendix_h_interactive_test.go H.17 |
| TERM-007 | pass | waived_closeout_test.go TestTERM007_ShortWriteDisablesInteractive |
| TERM-008 | pass | terminal/ansi_test.go cursor |
| TERM-009 | pass | release_matrix_test.go TestTERM009_CancelCleanupPath (SIGINT→Cancel library path; full PTY host-depe |
| TERM-010 | pass | docs: SIGKILL no guarantee (architecture spec + SECURITY.md) |
| TERM-011 | pass | port_env_test.go |
| TERM-012 | pass | port_env_test.go |
| TERM-013 | pass | frame coalesce H.22 |
| TERM-014 | pass | agent/review.Transcript NUL detector + review_test.go |
| TERM-015 | pass | slog_test.go |
| TERM-016 | pass | last_push_test.go |
| TERM-017 | pass | port_env_test.go |
| TERM-018 | pass | appendix_h_interactive_test.go H.20 |
| TERM-019 | pass | appendix_h_interactive_test.go H.21 |
| TERM-020 | pass | final_matrix_test.go |
| TERM-021 | pass | closeout_test.go |
| TERM-022 | pass | terminal/ansi.go |
| TERM-023 | pass | last_push_test.go |
| TERM-024 | pass | closeout_test.go |
| TXT-001 | pass | coverage_matrix_test.go |
| TXT-002 | pass | internal/width |
| TXT-003 | pass | internal/width CJK |
| TXT-004 | pass | internal/width emoji |
| TXT-005 | pass | internal/width ZWJ |
| TXT-006 | pass | sanitize |
| TXT-007 | pass | sanitize_test.go |
| TXT-008 | pass | final_matrix_test.go |
| TXT-009 | pass | final_matrix_test.go |
| TXT-010 | pass | final_matrix_test.go |
| TXT-011 | pass | plain narrow |
| TXT-012 | pass | closeout_test.go |
| TXT-013 | pass | waived_closeout_test.go TestTXT013_ANSIWidthParity |
| TXT-014 | pass | waived_closeout_test.go TestTXT014_OSC8ZeroCells |
| TXT-015 | pass | waived_closeout_test.go TestTXT015_NarrowStackDetailParent |
| TXT-016 | pass | waived_closeout_test.go TestTXT016_LeaderBoundedAndOmittedNarrow |
| TXT-017 | pass | closeout_test.go |
| TXT-018 | pass | final_matrix bidi |
| TXT-019 | pass | closeout_test.go |
| TXT-020 | pass | final_matrix_test.go |
