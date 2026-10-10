package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// handleReview serves evident_output_review.
func handleReview(id any, args map[string]any, cancelled *atomic.Bool) {
	src, _ := args["source"].(string)
	file, _ := args["file"].(string)
	kind, _ := args["kind"].(string)
	if kind == "directory" {
		handleReviewDirectory(id, args, cancelled)
		return
	}
	if file == "" {
		file = "input.go"
	}
	// MCP-036: remote paths unsupported — accept inlined content, or a
	// readable local absolute path.
	if isRemotePath(file) {
		writeRPC(id, toolError("remote path unsupported; pass source content only (MCP-036)"))
		return
	}
	if src == "" && filepath.IsAbs(file) {
		read, err := os.ReadFile(file)
		if err != nil {
			writeRPC(id, toolError(fmt.Sprintf("cannot read %s: %s", file, err)))
			return
		}
		src = string(read)
	}
	res, errMsg := reviewSource(args, kind, file, src)
	if errMsg != "" {
		writeRPC(id, toolError(errMsg))
		return
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeReviewResult(id, res)
}

// reviewSource reviews one non-directory input by kind. Go code is linted
// as the dialect of where it lives (review.DialectFor), the same owner
// kind=directory uses. A non-empty message is the tool error.
func reviewSource(args map[string]any, kind, file, src string) (review.Result, string) {
	desired, _ := args["desired_version"].(string)
	switch kind {
	case "transcript":
		return review.DialectFor(file, desired).Stamp(review.Transcript(file, src)), ""
	case "json", "structured":
		return review.DialectFor(file, desired).Stamp(review.StructuredDocument(file, []byte(src))), ""
	case "package":
		files, location, errMsg := packageFiles(args, file, src)
		if errMsg != "" {
			return review.Result{}, errMsg
		}
		dialect := review.DialectFor(location, desired)
		return dialect.Stamp(review.GoPackageAt(files, dialect.Lint())), ""
	default:
		if src == "" {
			return review.Result{}, "no source to review: pass `source` content or an absolute `file` path that exists"
		}
		dialect := review.DialectFor(file, desired)
		return dialect.Stamp(review.GoSourceAt(file, src, dialect.Lint())), ""
	}
}

// packageFiles resolves the package kind's files (MCP-017): the `files`
// map when given, else the single file/source pair. Each map value may be
// inline source text or a readable local absolute path, resolved the same
// way as the single `file` form. location is where the package lives for
// dialect resolution: the first absolute path in name order, else file.
// A non-empty message is the tool error.
func packageFiles(args map[string]any, file, src string) (files map[string]string, location, errMsg string) {
	files = map[string]string{file: src}
	location = file
	if raw, ok := args["files"].(map[string]any); ok {
		files = map[string]string{}
		location = ""
		for _, k := range slices.Sorted(maps.Keys(raw)) {
			s, ok := raw[k].(string)
			if !ok {
				continue
			}
			if isRemotePath(s) {
				return nil, "", "remote path unsupported; pass source content only (MCP-036)"
			}
			if filepath.IsAbs(s) {
				read, err := os.ReadFile(s)
				if err != nil {
					return nil, "", fmt.Sprintf("cannot read %s: %s", s, err)
				}
				if location == "" {
					location = s
				}
				s = string(read)
			}
			files[k] = s
		}
	}
	if allFileContentEmpty(files) {
		return nil, "", "empty source after decode: check files map shape"
	}
	return files, location, ""
}

// allFileContentEmpty reports whether every entry in a package-kind `files`
// map decoded to no usable content — e.g. the map held non-string values, or
// resolved paths read as empty. This is the honest diagnosis for the "empty
// source" failure mode: a generic parser EOF error tells the caller nothing
// about which of these two shapes actually happened.
func allFileContentEmpty(files map[string]string) bool {
	for _, content := range files {
		if content != "" {
			return false
		}
	}
	return true
}

// handleReviewDirectory serves evident_output_review with kind=directory.
func handleReviewDirectory(id any, args map[string]any, cancelled *atomic.Bool) {
	directory, _ := args["directory"].(string)
	if directory == "" {
		writeRPC(id, toolError("directory is required"))
		return
	}
	if isRemotePath(directory) {
		writeRPC(id, toolError("remote path unsupported; pass a local directory (MCP-036)"))
		return
	}
	if !filepath.IsAbs(directory) {
		writeRPC(id, toolError("directory must be an absolute local path"))
		return
	}
	desired, _ := args["desired_version"].(string)
	res, err := review.GoDirectoryAt(directory, desired)
	if err != nil {
		writeRPC(id, toolError("review directory: "+err.Error()))
		return
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	writeReviewResult(id, res)
}
