package review_test

// migrationFixture is one removed-name's dirty 1.0 shape and the canonical
// 1.1 rewrite. The contract test loads class=removed names from
// testdata/api_vocabulary.txt and requires a fixture per name.
type migrationFixture struct {
	dirty, clean string
}

func evoBody(body string) string {
	return "package p\nimport evo \"github.com/zachbornheimer/evident-output\"\n" + body
}

func evoIOBody(body string) string {
	return "package p\nimport (\n\t\"io\"\n\t\"os\"\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n" + body
}

func migration1_1Fixtures() map[string]migrationFixture {
	m := map[string]migrationFixture{
		"TaskHandle.Failf": {
			dirty: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Failf(\"boom\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Fail(\"boom\")\n}\n"),
		},
		"Output.Failf": {
			dirty: evoBody("func f(out *evo.Output) {\n\tout.Failf(\"boom\")\n}\n"),
			clean: evoBody("func f(out *evo.Output) {\n\tout.Fail(\"boom\")\n}\n"),
		},
		"TaskHandle.Blockf": {
			dirty: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Blockf(\"refused\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Block(\"refused\")\n}\n"),
		},
		"Warn": {
			dirty: evoBody("func f() {\n\tevo.Warn(\"stale entry ignored\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Problem(\"stale entry ignored\", evo.Severity(evo.SeverityWarning))\n}\n"),
		},
		"Output.Warn": {
			dirty: evoBody("func f(out *evo.Output) {\n\tout.Warn(\"stale entry ignored\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Problem(\"stale entry ignored\", evo.Severity(evo.SeverityWarning))\n}\n"),
		},
		"TaskHandle.Warn": {
			dirty: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Warn(\"stale entry ignored\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Problem(\"stale entry ignored\", evo.Severity(evo.SeverityWarning))\n}\n"),
		},
		"TaskHandle.Step": {
			dirty: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Step(1, 3, \"file.go\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Progress(1, 3).Doing(\"file.go\")\n}\n"),
		},
		"TaskHandle.Kept": {
			dirty: evoBody("var reasonProtected = evo.Reason(\"protected\")\nfunc f(task *evo.TaskHandle) {\n\ttask.Kept(reasonProtected)\n}\n"),
			clean: evoBody("var reasonProtected = evo.Reason(\"protected\")\nfunc f(task *evo.TaskHandle) {\n\ttask.Skipped(reasonProtected)\n}\n"),
		},
		"Evidence": {
			dirty: evoBody("func f(task *evo.TaskHandle) {\n\t_ = task.Evidence()\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\t_ = task.Capture()\n}\n"),
		},
		"Failure.Next": {
			dirty: evoBody("func f(fail *evo.Failure) {\n\tfail.Next(evo.Label(\"retry\"))\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\ttask.Next(evo.Label(\"retry\"))\n}\n"),
		},
		"Failure.NextCommand": {
			dirty: evoBody("func f(fail *evo.Failure) {\n\tfail.NextCommand(\"git\", \"status\")\n}\n"),
			clean: evoBody("func f(task *evo.TaskHandle) {\n\ttask.Fail(\"failed\")\n\ttask.NextCommand(\"git\", \"status\")\n}\n"),
		},
		"EncodeJSON": {
			dirty: evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeJSON(w, r)\n}\n"),
			clean: evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.WriteJSON(w, r)\n}\n"),
		},
		"EncodeJSONL": {
			dirty: evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeJSONL(w, r)\n}\n"),
			clean: evoBody("func f() {\n\t_ = evo.Init(evo.Config{Format: evo.FormatJSONL})\n}\n"),
		},
		"EncodeEventJSON": {
			dirty: evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeEventJSON(w, r)\n}\n"),
			clean: evoBody("func f() {\n\t_ = evo.Init(evo.Config{Format: evo.FormatJSONL})\n}\n"),
		},
		"ForSkip": {
			dirty: evoBody("func f() {\n\t_ = evo.ForSkip()\n}\n"),
			clean: evoBody("func f() {\n\t_ = evo.Reason(\"skip\")\n}\n"),
		},
		"OnTask": {
			dirty: evoBody("func f() {\n\t_ = evo.OnTask()\n}\n"),
			clean: evoBody("func f() {\n\t_ = evo.Reason(\"skip\")\n}\n"),
		},
		"EventSchemaVersion": {
			dirty: evoBody("func f() {\n\t_ = evo.EventSchemaVersion\n}\n"),
			clean: evoBody("func f() {\n\t_ = evo.Init()\n}\n"),
		},
	}
	addOptionFixtures(m)
	addTypeFixtures(m)
	return m
}

func addOptionFixtures(m map[string]migrationFixture) {
	type opt struct {
		name, params, call, field string
	}
	for _, o := range []opt{
		{"To", "w io.Writer", "evo.To(w)", "Stdout: w"},
		{"AlsoWrite", "w io.Writer", "evo.AlsoWrite(w)", "Stdout: io.MultiWriter(os.Stdout, w)"},
		{"Plain", "", "evo.Plain()", "Plain: true"},
		{"NoColor", "", "evo.NoColor()", "Color: evo.ColorNever"},
		{"Stdin", "r io.Reader", "evo.Stdin(r)", "Stdin: r"},
		{"DryRun", "", "evo.DryRun()", "DryRun: true"},
		{"VisibilityDelay", "", "evo.VisibilityDelay(0)", "VisibilityDelay: evo.Delay(0)"},
		{"Diagnostics", "w io.Writer", "evo.Diagnostics(w)", "Stderr: w"},
		{"Title", "", "evo.Title(\"tool\")", "Title: \"tool\""},
		{"ResultStream", "w io.Writer", "evo.ResultStream(w)", "Result: w"},
		{"Terminal", "drv evo.TerminalDriver", "evo.Terminal(drv)", "Terminal: drv"},
		{"Clock", "c evo.TimeSource", "evo.Clock(c)", "Clock: c"},
		{"MaxFrameRate", "", "evo.MaxFrameRate(20)", "MaxFrameRate: 20"},
		{"Width", "", "evo.Width(80)", "Width: 80"},
		{"Redact", "r evo.Redactor", "evo.Redact(r)", "Redactor: r"},
		{"Runner", "r evo.ProcessRunner", "evo.Runner(r)", "ProcessRunner: r"},
		{"MaxEntities", "", "evo.MaxEntities(10)", "MaxEntities: 10"},
		{"MaxEvents", "", "evo.MaxEvents(10)", "MaxEvents: 10"},
		{"Strict", "", "evo.Strict()", "Strict: true"},
		{"Glyphs", "", "evo.Glyphs(evo.GlyphsASCII)", "Glyphs: evo.GlyphsASCII"},
		{"DataProjection", "", "evo.DataProjection()", "Format: evo.FormatData"},
		{"ExternalProjection", "", "evo.ExternalProjection()", "Format: evo.FormatExternal"},
		{"DebugLevel", "", "evo.DebugLevel(evo.LevelDebug)", "Debug: evo.DebugConfig{Level: evo.LevelDebug}"},
		{"DebugAddSource", "", "evo.DebugAddSource()", "Debug: evo.DebugConfig{AddSource: true}"},
		{"DebugHistory", "", "evo.DebugHistory()", "Debug: evo.DebugConfig{View: evo.DebugPresentationHistory}"},
		{"DebugPane", "", "evo.DebugPane()", "Debug: evo.DebugConfig{View: evo.DebugPresentationPane}"},
	} {
		fn := "func f() {\n"
		if o.params != "" {
			fn = "func f(" + o.params + ") {\n"
		}
		m[o.name] = migrationFixture{
			dirty: evoIOBody(fn + "\t_ = " + o.call + "\n}\n"),
			clean: evoIOBody(fn + "\t_ = evo.Init(evo.Config{" + o.field + "})\n}\n"),
		}
	}
}

func addTypeFixtures(m map[string]migrationFixture) {
	type types struct {
		name, next string
	}
	for _, t := range []types{
		{"EvidenceOption", "CaptureOption"},
		{"EvidenceStream", "CaptureStream"},
		{"EvidenceStreamCombined", "CaptureStreamCombined"},
		{"EvidenceStreamStdout", "CaptureStreamStdout"},
		{"EvidenceStreamStderr", "CaptureStreamStderr"},
		{"MaxEvidenceBytes", "MaxCaptureBytes"},
		{"Option", "Config"},
		{"ReasonOption", "Reason"},
		{"ErrReasonSkipOnly", "Reason"},
		{"ErrReasonWrongTask", "Reason"},
		{"ConclusionJSON", "WriteJSON"},
		{"EventJSON", "FormatJSONL"},
		{"JSONAction", "WriteJSON"},
		{"JSONChanges", "WriteJSON"},
		{"JSONCollection", "WriteJSON"},
		{"JSONCommand", "WriteJSON"},
		{"JSONDocument", "WriteJSON"},
		{"JSONEffectRecord", "WriteJSON"},
		{"JSONMessage", "WriteJSON"},
		{"JSONOutputMeta", "WriteJSON"},
		{"JSONPlan", "WriteJSON"},
		{"JSONProblem", "WriteJSON"},
		{"JSONProgress", "WriteJSON"},
		{"JSONSchemaVersion", "WriteJSON"},
		{"JSONTask", "WriteJSON"},
	} {
		assign := "var _ evo."
		cleanAssign := "var _ evo."
		if t.name == "ErrReasonSkipOnly" || t.name == "ErrReasonWrongTask" ||
			t.name == "EvidenceStreamCombined" || t.name == "EvidenceStreamStdout" ||
			t.name == "EvidenceStreamStderr" || t.name == "EventJSON" ||
			t.name == "ConclusionJSON" {
			assign = "_ = evo."
			cleanAssign = "_ = evo."
		}
		m[t.name] = migrationFixture{
			dirty: evoBody("func f() {\n\t" + assign + t.name + "\n}\n"),
			clean: evoBody("func f() {\n\t" + cleanAssign + t.next + "\n}\n"),
		}
	}
}
