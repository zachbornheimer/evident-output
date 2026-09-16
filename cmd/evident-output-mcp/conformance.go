package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// handleConformanceTool answers spec §60's conformance questions (is this
// Evo usage current, what is stale/unsafe/upgradable, what provenance is
// explicit vs opaque) by running the same static review evident_output_review
// uses and reshaping the result to the conformance wire shape. It accepts a
// single Go source (source/file) or a local directory (kind=directory) —
// the same two review inputs a version-aware upgrade sweep needs.
func handleConformanceTool(id any, args map[string]any, cancelled *atomic.Bool) {
	res, err := conformanceReviewResult(args)
	if err != nil {
		writeRPC(id, toolError(err.Error()))
		return
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	applyDesiredVersion(&res, args)
	targetVersion, _ := args["target_version"].(string)
	writeConformanceResult(id, review.Conformance(res, targetVersion))
}

// conformanceReviewResult resolves the same source/file/directory inputs
// evident_output_review accepts into one review.Result.
func conformanceReviewResult(args map[string]any) (review.Result, error) {
	if directory, _ := args["directory"].(string); directory != "" || args["kind"] == "directory" {
		return conformanceDirectoryResult(args)
	}
	src, _ := args["source"].(string)
	file, _ := args["file"].(string)
	if file == "" {
		file = "input.go"
	}
	if isRemotePath(file) {
		return review.Result{}, fmt.Errorf("remote path unsupported; pass source content only (MCP-036)")
	}
	if src == "" && filepath.IsAbs(file) {
		read, err := os.ReadFile(file)
		if err != nil {
			return review.Result{}, fmt.Errorf("cannot read %s: %w", file, err)
		}
		src = string(read)
	}
	if src == "" {
		return review.Result{}, fmt.Errorf("no source to review: pass `source` content or an absolute `file` path that exists")
	}
	desired, _ := args["desired_version"].(string)
	return review.GoSourceAt(file, src, desired), nil
}

func conformanceDirectoryResult(args map[string]any) (review.Result, error) {
	directory, _ := args["directory"].(string)
	if directory == "" {
		return review.Result{}, fmt.Errorf("directory is required")
	}
	if isRemotePath(directory) {
		return review.Result{}, fmt.Errorf("remote path unsupported; pass a local directory (MCP-036)")
	}
	if !filepath.IsAbs(directory) {
		return review.Result{}, fmt.Errorf("directory must be an absolute local path")
	}
	desired, _ := args["desired_version"].(string)
	res, err := review.GoDirectoryAt(directory, desired)
	if err != nil {
		return review.Result{}, fmt.Errorf("conformance directory: %w", err)
	}
	return res, nil
}

// writeConformanceResult writes the spec §60 shape:
// {target_version, findings[{rule,severity,file,line,summary,migration}]}.
func writeConformanceResult(id any, report review.ConformanceReport) {
	text := fmt.Sprintf("target_version=%s findings=%d", report.TargetVersion, len(report.Findings))
	writeRPC(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"structuredContent": map[string]any{
			"schema":         "evident_output_conformance.v1",
			"target_version": report.TargetVersion,
			"findings":       report.Findings,
		}})
}
