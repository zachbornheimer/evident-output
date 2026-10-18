package review

import (
	"go/ast"
	"strings"
)

// The v1.0.0 Config fields as docs/migration/2.0.md maps them ("Construction"
// table): a rename, a field of the Facades or State struct, a Run method, or
// nothing at all.

// configKeyRenames are the Config fields 2.0 keeps under another name.
var configKeyRenames = map[string]string{"Title": "Name", "Preview": "DryRun"}

// configKeyMove is a v1.0.0 Config field that is a field of a 2.0 struct.
type configKeyMove struct{ group, field string }

var configKeyMoves = map[string]configKeyMove{
	"Stdin": {"Facades", "Stdin"}, "Stdout": {"Facades", "Stdout"}, "Stderr": {"Facades", "Stderr"},
	"Clock": {"Facades", "Clock"}, "Redactor": {"Facades", "Redactor"}, "Terminal": {"Facades", "Terminal"},
	"StateDir": {"State", "Dir"}, "AppID": {"State", "AppID"},
}

// configKeyGone are the v1.0.0 Config fields with no 2.0 field, each with
// what replaces it; the rewrite cannot place the value, so the finding is
// guidance.
var configKeyGone = map[string]struct{ message, guidance string }{
	"Subject": {
		"a durable line under the title is Run.Fact(name, value)",
		`add r.Fact("subject", <value>) as the first statement of run`,
	},
	"Facts": {
		"run facts are Run.Fact(name, value)",
		"add one r.Fact(name, value) per entry as the first statements of run",
	},
	"Color": {
		"colour follows the NO_COLOR and FORCE_COLOR environment variables",
		"remove the field and set NO_COLOR or FORCE_COLOR in the environment instead",
	},
	"FailedExitCode": {
		"a Failed run always exits 2",
		"remove the field and map exit code 2 in the caller when it must be another",
	},
	"Isolated": {
		"Main and Execute install no package-level default, so every Run is isolated",
		"remove the field; nothing replaces it",
	},
}

// configKeys is the Config literal's fields by key.
func configKeys(cl *ast.CompositeLit) map[string]*ast.KeyValueExpr {
	keys := map[string]*ast.KeyValueExpr{}
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			keys[identName(kv.Key)] = kv
		}
	}
	return keys
}

// groupedConfigFields is the Facades and State struct literals a Config
// literal's moved fields make, in the order the fields appear.
func (d *removedTwoZeroDetector) groupedConfigFields(cl *ast.CompositeLit) []string {
	groups := map[string][]string{}
	var order []string
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if move, moved := configKeyMoves[identName(kv.Key)]; moved {
			if groups[move.group] == nil {
				order = append(order, move.group)
			}
			groups[move.group] = append(groups[move.group], move.field+": "+d.nodeSrc(kv.Value))
		}
	}
	out := make([]string, len(order))
	for i, group := range order {
		out[i] = group + ": " + d.pkg + "." + group + "{" + strings.Join(groups[group], ", ") + "}"
	}
	return out
}

// reportConfigFields words the findings a standalone Config literal's
// removed fields get: a rename or a lone moved field is a rewrite, and a
// field with no 2.0 field is guidance.
func (d *removedTwoZeroDetector) reportConfigFields(cl *ast.CompositeLit) {
	keys := configKeys(cl)
	moved := map[string]int{}
	for key := range keys {
		if move, ok := configKeyMoves[key]; ok {
			moved[move.group]++
		}
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, old := identName(kv.Key), d.nodeSrc(kv)
		switch move, isMove := configKeyMoves[key]; {
		case configKeyRenames[key] != "" && key != "Title" && keys[configKeyRenames[key]] == nil:
			d.reportConfigField(kv, key, "the field is Config."+configKeyRenames[key], "replace "+old+" with "+configKeyRenames[key]+": "+d.nodeSrc(kv.Value))
		case isMove && moved[move.group] == 1:
			d.reportConfigField(kv, key, "the field is Config."+move.group+"."+move.field,
				"replace "+old+" with "+move.group+": "+d.pkg+"."+move.group+"{"+move.field+": "+d.nodeSrc(kv.Value)+"}")
		case isMove:
			d.reportConfigField(kv, key, "the field is Config."+move.group+"."+move.field,
				"group the "+move.group+" fields of this Config into one "+move.group+": "+d.pkg+"."+move.group+"{"+move.field+": ...}")
		}
	}
	d.reportDroppedConfigFields(cl)
}

// reportDroppedConfigFields is the guidance for the Config fields 2.0 has no
// field for. The Config of a removed Init or New (removed in 2.0) reports only
// these: the rest of the literal moves into the Config of Main or Execute.
func (d *removedTwoZeroDetector) reportDroppedConfigFields(cl *ast.CompositeLit) {
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if gone, dropped := configKeyGone[identName(kv.Key)]; dropped {
				d.reportConfigField(kv, identName(kv.Key), gone.message, strings.Replace(gone.guidance, "<value>", d.nodeSrc(kv.Value), 1))
			}
		}
	}
}

func (d *removedTwoZeroDetector) reportConfigField(kv *ast.KeyValueExpr, key, instead, suggestion string) {
	d.report(kv, "Config."+key+" was removed in 2.0; "+instead, suggestion)
	d.cover(kv)
}
