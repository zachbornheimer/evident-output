package wire

import "github.com/zachbornheimer/evident-output/internal/core"

// ProblemDoc is a wire-format Problem: stable code plus a human message a
// machine consumer must not parse (spec §37). It carries every
// core.Problem field an exported ProblemOption can set, so the plain/TTY
// row never shows machine truth this document drops (ZYS-823).
type ProblemDoc struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Subject string `json:"subject,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// CaptureTail's wire key stays "evidence_tail": a documented wire-compat
	// decision (1.1 vocabulary freeze, E-121) — the Go field followed the
	// capture-meaning Evidence* rename, but run.v2 payloads already on disk
	// use this key, and the schema itself is not part of that rename.
	CaptureTail string       `json:"evidence_tail,omitempty"`
	Count       int64        `json:"count,omitempty"`
	Unit        string       `json:"unit,omitempty"`
	Location    *LocationDoc `json:"location,omitempty"`
	Remedies    []ActionDoc  `json:"remedies,omitempty"`
}

// LocationDoc is a wire-format source position.
type LocationDoc struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// ActionDoc is a wire-format next-step Action.
type ActionDoc struct {
	Label   string      `json:"label,omitempty"`
	Command *CommandDoc `json:"command,omitempty"`
	URL     string      `json:"url,omitempty"`
}

// CommandDoc is argv for display.
type CommandDoc struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

// ToProblemDoc is the one core.Problem projection: evo.run's task problems
// and the JSONL problem.recorded/warning.recorded payloads both build from
// it.
func ToProblemDoc(p core.Problem) ProblemDoc {
	return ProblemDoc{
		Code: p.Code, Message: p.Summary, Subject: p.Subject,
		Detail: p.Detail, CaptureTail: p.CaptureTail,
		Count: p.Count, Unit: p.Unit,
		Location: toLocationDoc(p.Location),
		Remedies: toActionDocs(p.Actions),
	}
}

// ToActionDoc is the one core.Action projection, shared by evo.run and
// the output.v1 renderer.
func ToActionDoc(a core.Action) ActionDoc {
	ad := ActionDoc{Label: a.Label, URL: a.URL}
	if a.Command != nil {
		ad.Command = &CommandDoc{
			Executable: a.Command.Executable,
			Args:       append([]string(nil), a.Command.Args...),
		}
	}
	return ad
}

func toProblemDocs(in []core.Problem) []ProblemDoc {
	out := make([]ProblemDoc, 0, len(in))
	for _, p := range in {
		out = append(out, ToProblemDoc(p))
	}
	return out
}

func toLocationDoc(loc *core.SourceLocation) *LocationDoc {
	if loc == nil {
		return nil
	}
	return &LocationDoc{Path: loc.Path, Line: loc.Line, Column: loc.Column}
}

func toActionDocs(in []core.Action) []ActionDoc {
	if len(in) == 0 {
		return nil
	}
	out := make([]ActionDoc, len(in))
	for i, a := range in {
		out[i] = ToActionDoc(a)
	}
	return out
}
