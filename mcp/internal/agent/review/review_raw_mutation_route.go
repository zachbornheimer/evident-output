package review

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// evoFSDeletionCallNames are the raw calls that remove filesystem state —
// the mutations evo.Effect with EffectSpec.Path (a filesystem claim) owns.
// evo.File cannot express a deletion, so API-057 never asks for one.
var evoFSDeletionCallNames = map[string]bool{"os.Remove": true, "os.RemoveAll": true}

// rawMutationSite is one raw mutation call inside a Define callback, with
// what a rewrite of that call needs: the source it is cut from, the evo
// import's local name, and the callback's context parameter (empty when the
// callback does not name one).
type rawMutationSite struct {
	mutation rawMutationCall
	src      string
	fset     *token.FileSet
	pkg      string
	ctx      string
}

// lineBreaks matches a line break and its indentation.
var lineBreaks = regexp.MustCompile(`\s*\n\s*`)

// text returns the source of node on one line, because a Suggestion is one
// line and the rewrite grammar reads a space as any whitespace run.
func (s rawMutationSite) text(node ast.Node) string {
	return lineBreaks.ReplaceAllString(s.src[s.fset.Position(node.Pos()).Offset:s.fset.Position(node.End()).Offset], " ")
}

// rawMutationRoute names the Evo boundary that owns the site's kind of
// mutation: a database handle's Exec or a filesystem deletion goes through
// evo.Effect (neither File nor Exec can express it as a counted change);
// writes and processes go through File/Exec. A deletion whose callback names
// its context gets the exact rewrite ("replace CALL with evo.Effect(...)"),
// so a reviewer can apply it and review again.
func rawMutationRoute(site rawMutationSite) string {
	name := site.mutation.name
	if strings.HasSuffix(name, ".Exec") || strings.HasSuffix(name, ".ExecContext") {
		if !strings.HasPrefix(name, "exec.") {
			return "wrap " + name + " in evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: \"row\", Quantity: n}, fn) so dry-run never invokes it"
		}
	}
	if evoFSDeletionCallNames[name] {
		return deletionRoute(site)
	}
	return "route the mutation through evo.File(ctx, evo.FileSpec{...}) or evo.Exec(ctx, evo.ExecSpec{...}) instead of calling " + name + " directly"
}

// deletionRoute is the Effect that claims a deleted path.
func deletionRoute(site rawMutationSite) string {
	call := site.mutation.call
	if site.ctx == "" || len(call.Args) == 0 {
		return "wrap " + site.mutation.name + " in evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: \"file\", Quantity: 1, Path: path}, fn) so dry-run never invokes it; name the Define callback's context parameter first"
	}
	callText := site.text(call)
	spec := site.pkg + ".EffectSpec{Verb: " + site.pkg + ".Delete, Object: \"file\", Quantity: 1, Path: " + site.text(call.Args[0]) + "}"
	return "replace " + callText + " with " + site.pkg + ".Effect(" + site.ctx + ", " + spec + ", func(context.Context) error { return " + callText + " })"
}

// callbackContextName is the name of fn's first parameter, or "" when it is
// unnamed or blank (so the body has no context to hand to Effect).
func callbackContextName(fn *ast.FuncLit) string {
	params := fn.Type.Params.List
	if len(params) == 0 || len(params[0].Names) == 0 || params[0].Names[0].Name == "_" {
		return ""
	}
	return params[0].Names[0].Name
}
