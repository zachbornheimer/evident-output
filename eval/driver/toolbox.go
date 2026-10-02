package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// Tool names.
const (
	toolReview  = "evident_output_review"
	toolExplain = "evident_output_explain"
	toolPreview = "evident_output_preview"
	toolGoBuild = "go_build"
	toolSubmit  = "submit"
)

// forwardedMCPTools are the only MCP tools a model may call. adopt_plan,
// conformance and update are left out: they read the host filesystem or
// reinstall binaries.
var forwardedMCPTools = []string{toolListSections, toolGetDocumentation, toolReview, toolExplain, toolPreview}

// hostPathArguments name arguments that would let a tool read the host.
var hostPathArguments = []string{"directory", "file"}

const filesSchema = `{"type":"object","properties":{"files":{"type":"object","additionalProperties":{"type":"string"},"description":"File name (a plain .go name, package main) to full file contents."}},"required":["files"]}`

// Outcome is the result of one tool call.
type Outcome struct {
	Text    string
	IsError bool
	// Submission is set by submit and ends the sample.
	Submission map[string]string
	// ReviewClean is set by review calls: true when recheck_required=false.
	ReviewClean *bool
	// Built is set by go_build.
	Built bool
}

// Toolbox is everything a model can touch: the forwarded MCP tools, go_build
// and submit. It has no repository, web, or shell access.
type Toolbox struct {
	Server  ToolServer
	Builder Builder
}

// Definitions lists the tool definitions, forwarded MCP tools first and
// submit last (the prompt-cache breakpoint lands on the last tool).
func (t Toolbox) Definitions(ctx context.Context) ([]ToolDef, error) {
	all, err := t.Server.Tools(ctx)
	if err != nil {
		return nil, err
	}
	var defs []ToolDef
	for _, name := range forwardedMCPTools {
		index := slices.IndexFunc(all, func(def ToolDef) bool { return def.Name == name })
		if index < 0 {
			return nil, fmt.Errorf("list tools: MCP server does not offer %s", name)
		}
		defs = append(defs, all[index])
	}
	defs = append(defs,
		ToolDef{Name: toolGoBuild, Description: "Build the candidate files against the evo library in a scratch module and return the compiler output.", InputSchema: json.RawMessage(filesSchema)},
		ToolDef{Name: toolSubmit, Description: "Submit the final candidate files. This ends the task; call it once, when done.", InputSchema: json.RawMessage(filesSchema)},
	)
	return defs, nil
}

// Dispatch runs one tool call. Failures the model can act on come back as an
// error Outcome; only infrastructure faults return an error.
func (t Toolbox) Dispatch(ctx context.Context, use Block) (Outcome, error) {
	switch {
	case use.ToolName == toolGoBuild:
		return t.goBuild(ctx, use.Input)
	case use.ToolName == toolSubmit:
		return submit(use.Input), nil
	case slices.Contains(forwardedMCPTools, use.ToolName):
		return t.forward(ctx, use)
	default:
		return Outcome{Text: fmt.Sprintf("unknown tool %q", use.ToolName), IsError: true}, nil
	}
}

func parseFiles(input json.RawMessage) (map[string]string, error) {
	var args struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parse tool input: %w", err)
	}
	if err := ValidateFiles(args.Files); err != nil {
		return nil, err
	}
	return args.Files, nil
}

func submit(input json.RawMessage) Outcome {
	files, err := parseFiles(input)
	if err != nil {
		return Outcome{Text: err.Error(), IsError: true}
	}
	return Outcome{Text: "submitted", Submission: files}
}

func (t Toolbox) goBuild(ctx context.Context, input json.RawMessage) (Outcome, error) {
	files, err := parseFiles(input)
	if err != nil {
		return Outcome{Text: err.Error(), IsError: true}, nil
	}
	output, compiled, err := t.Builder.Build(ctx, files)
	if err != nil {
		return Outcome{}, err
	}
	if compiled {
		return Outcome{Text: "build ok", Built: true}, nil
	}
	return Outcome{Text: output, IsError: true}, nil
}

func (t Toolbox) forward(ctx context.Context, use Block) (Outcome, error) {
	if blocked := hostPathArgument(use.Input); blocked != "" {
		return Outcome{Text: fmt.Sprintf("argument %q is not available: pass Go source or files instead", blocked), IsError: true}, nil
	}
	result, err := t.Server.Call(ctx, use.ToolName, use.Input)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{Text: result.Render(), IsError: result.IsError}
	if use.ToolName == toolReview && !result.IsError {
		outcome.ReviewClean = reviewClean(result.Structured)
	}
	return outcome, nil
}

func hostPathArgument(input json.RawMessage) string {
	var args map[string]json.RawMessage
	if json.Unmarshal(input, &args) != nil {
		return ""
	}
	for _, name := range hostPathArguments {
		if _, present := args[name]; present {
			return name
		}
	}
	return ""
}

func reviewClean(structured json.RawMessage) *bool {
	var result struct {
		RecheckRequired *bool `json:"recheck_required"`
	}
	if json.Unmarshal(structured, &result) != nil || result.RecheckRequired == nil {
		return nil
	}
	clean := !*result.RecheckRequired
	return &clean
}
