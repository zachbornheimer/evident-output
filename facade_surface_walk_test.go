package evo_test

import (
	"fmt"
	"go/token"
	"go/types"
	"path"
	"slices"
	"strings"
)

// systemType names a system type by import path and type name.
type systemType struct{ importPath, name string }

func (s systemType) label() string { return path.Base(s.importPath) + "." + s.name }

// bannedSystemTypes are the system types an exported facade surface must not
// reach: a caller holding one could chmod a raw handle, signal a raw process
// or rewire a raw command. Every one is matched by identity with go/types, so
// an alias, a defined type of it, a struct field, an embedding or an interface
// method returning it is caught as surely as a direct signature.
//
// os.FileInfo (= io/fs.FileInfo) is deliberately absent: it is a pure value
// interface (name, size, mode, time) with no handle behind it, and fs.Stat and
// fs.Lstat return it.
var bannedSystemTypes = []systemType{
	{"os", "File"},
	{"os", "Process"},
	{"os", "Signal"},
	{"os/exec", "Cmd"},
}

// exemptAlias is the one alias of a banned type a facade may export:
// process.Signal stays os.Signal because callers only pass it to Notify and
// StopNotify. Every other alias of a banned type must be wrapped instead.
var exemptAlias = systemType{modulePath + "/internal/process", "Signal"}

// resolvedSystemType is a banned type resolved to its go/types object.
type resolvedSystemType struct {
	label string
	named *types.Named
}

// surfaceWalker collects the banned system types reachable from the exported
// surface of a type-checked package.
type surfaceWalker struct {
	fset   *token.FileSet
	banned []resolvedSystemType
	seen   map[*types.Named]bool
	leaks  []string
}

func newSurfaceWalker(fset *token.FileSet, banned []resolvedSystemType) *surfaceWalker {
	return &surfaceWalker{fset: fset, banned: banned, seen: map[*types.Named]bool{}}
}

// leaksOf reports each way the exported surface of pkg reaches a banned type,
// sorted, as "file:line:col Owner exposes pkg.Type".
func (w *surfaceWalker) leaksOf(pkg *types.Package) []string {
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj.Exported() {
			w.walk(pkg.Name()+"."+name, obj.Pos(), obj.Type())
		}
	}
	slices.Sort(w.leaks)
	return w.leaks
}

func (w *surfaceWalker) report(owner string, pos token.Pos, what string) {
	w.leaks = append(w.leaks, fmt.Sprintf("%s %s exposes %s", w.fset.Position(pos), owner, what))
}

// walk follows t through every composite a caller can reach through.
func (w *surfaceWalker) walk(owner string, pos token.Pos, t types.Type) {
	switch t := t.(type) {
	case *types.Alias:
		if isExemptAlias(t) {
			return
		}
		w.walk(owner, pos, types.Unalias(t))
	case *types.Named:
		w.walkNamed(owner, pos, t)
	case *types.Pointer:
		w.walk(owner, pos, t.Elem())
	case *types.Slice:
		w.walk(owner, pos, t.Elem())
	case *types.Array:
		w.walk(owner, pos, t.Elem())
	case *types.Chan:
		w.walk(owner, pos, t.Elem())
	case *types.Map:
		w.walk(owner, pos, t.Key())
		w.walk(owner, pos, t.Elem())
	case *types.Signature:
		w.walkTuple(owner, pos, t.Params())
		w.walkTuple(owner, pos, t.Results())
	case *types.Struct:
		w.walkStruct(owner, t)
	case *types.Interface:
		w.walkInterface(owner, t)
	}
}

func (w *surfaceWalker) walkTuple(owner string, pos token.Pos, tuple *types.Tuple) {
	for v := range tuple.Variables() {
		w.walk(owner, pos, v.Type())
	}
}

// walkStruct follows each exported field and each embedded field, exported or
// not: an unexported embedded field still promotes its methods.
func (w *surfaceWalker) walkStruct(owner string, s *types.Struct) {
	for field := range s.Fields() {
		if field.Exported() || field.Embedded() {
			w.walk(owner+"."+field.Name(), field.Pos(), field.Type())
		}
	}
}

func (w *surfaceWalker) walkInterface(owner string, iface *types.Interface) {
	for method := range iface.Methods() {
		if method.Exported() {
			w.walk(owner+"."+method.Name(), method.Pos(), method.Type())
		}
	}
}

// walkNamed reports t if it is a banned type or a defined type of one, and
// otherwise follows a type of this module into its underlying type and its
// method set. A type of any other package is opaque: it is the facade's job
// to wrap it, not this walk's to audit it.
func (w *surfaceWalker) walkNamed(owner string, pos token.Pos, t *types.Named) {
	if banned, ok := w.bannedLabel(t); ok {
		w.report(owner, pos, banned)
		return
	}
	if !isModuleType(t) || w.seen[t] {
		return
	}
	w.seen[t] = true
	if definedFrom, ok := w.definedFromBanned(t); ok {
		w.report(owner, pos, "a defined type of "+definedFrom)
		return
	}
	own := t.Obj().Pkg().Name() + "." + t.Obj().Name()
	w.walk(own, t.Obj().Pos(), t.Underlying())
	w.walkMethods(own, t)
}

// walkMethods follows each exported method of t, promoted ones included.
// An interface's methods are followed through its underlying type instead.
func (w *surfaceWalker) walkMethods(owner string, t *types.Named) {
	if _, isInterface := t.Underlying().(*types.Interface); isInterface {
		return
	}
	methods := types.NewMethodSet(types.NewPointer(t))
	for sel := range methods.Methods() {
		if method := sel.Obj(); method.Exported() {
			w.walk(owner+"."+method.Name(), method.Pos(), method.Type())
		}
	}
}

func (w *surfaceWalker) bannedLabel(t *types.Named) (string, bool) {
	for _, b := range w.banned {
		if types.Identical(t, b.named) {
			return b.label, true
		}
	}
	return "", false
}

// definedFromBanned reports whether t is declared as `type T banned`, which
// shares the banned type's underlying type and so its handle.
func (w *surfaceWalker) definedFromBanned(t *types.Named) (string, bool) {
	for _, b := range w.banned {
		if types.Identical(t.Underlying(), b.named.Underlying()) {
			return b.label, true
		}
	}
	return "", false
}

func isExemptAlias(a *types.Alias) bool {
	obj := a.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == exemptAlias.importPath && obj.Name() == exemptAlias.name
}

func isModuleType(t *types.Named) bool {
	pkg := t.Obj().Pkg()
	return pkg != nil && strings.HasPrefix(pkg.Path(), modulePath+"/")
}
