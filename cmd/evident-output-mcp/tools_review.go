package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
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
	var res review.Result
	switch kind {
	case "transcript":
		res = review.Transcript(file, src)
	case "json", "structured":
		res = review.StructuredDocument(file, []byte(src))
	case "package":
		files, errMsg := packageFiles(args, file, src)
		if errMsg != "" {
			writeRPC(id, toolError(errMsg))
			return
		}
		res = review.GoPackage(files)
	default:
		if src == "" {
			writeRPC(id, toolError("no source to review: pass `source` content or an absolute `file` path that exists"))
			return
		}
		desired, _ := args["desired_version"].(string)
		res = review.GoSourceAt(file, src, desired)
	}
	if cancelled.Load() {
		writeRPC(id, toolError("deadline exceeded"))
		return
	}
	applyDesiredVersion(&res, args)
	writeReviewResult(id, res)
}

// packageFiles resolves the package kind's files (MCP-017): the `files`
// map when given, else the single file/source pair. Each map value may be
// inline source text or a readable local absolute path, resolved the same
// way as the single `file` form. A non-empty message is the tool error.
func packageFiles(args map[string]any, file, src string) (map[string]string, string) {
	files := map[string]string{file: src}
	if raw, ok := args["files"].(map[string]any); ok {
		files = map[string]string{}
		for k, v := range raw {
			s, ok := v.(string)
			if !ok {
				continue
			}
			if isRemotePath(s) {
				return nil, "remote path unsupported; pass source content only (MCP-036)"
			}
			if filepath.IsAbs(s) {
				read, err := os.ReadFile(s)
				if err != nil {
					return nil, fmt.Sprintf("cannot read %s: %s", s, err)
				}
				s = string(read)
			}
			files[k] = s
		}
	}
	if allFileContentEmpty(files) {
		return nil, "empty source after decode: check files map shape"
	}
	return files, ""
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
	applyDesiredVersion(&res, args)
	writeReviewResult(id, res)
}

func applyDesiredVersion(res *review.Result, args map[string]any) {
	if v, _ := args["desired_version"].(string); v != "" {
		res.DesiredVersion = v
	}
}
