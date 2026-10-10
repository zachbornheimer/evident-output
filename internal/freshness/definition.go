// This file owns operation definition fingerprints: the digest of everything
// that defines a File, an Exec or an opaque Task, so a changed definition is
// stale and an unchanged one can be current.

package freshness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
)

// Operation kinds a manifest records.
const (
	// OperationKindFile marks a File operation's record.
	OperationKindFile = "file"
	// OperationKindExec marks an Exec operation's record.
	OperationKindExec = "exec"
	// OutputKindExec marks an Exec's declared output record.
	OutputKindExec = "exec-output"
	// TaskBasisOperationKind marks the record that carries a Task's observed
	// Basis. It is always the last operation of the Task.
	TaskBasisOperationKind = "task_basis"
)

const (
	fileDefinitionMarker   = "evident-output:file:definition:v1\x00"
	execDefinitionMarker   = "evident-output:exec:definition:v1\x00"
	opaqueDefinitionMarker = "evident-output:task:definition:opaque:v1\x00"
	contentsManagedMarker  = "contents:managed\x00"
	contentsUnmanaged      = "contents:unmanaged\x00"
	definitionPrefix       = "sha256:"
)

// preimage builds the byte string a definition fingerprint hashes. Every
// text field ends in a NUL so adjacent fields can never run together.
type preimage struct{ h hash.Hash }

func newPreimage(marker string) preimage {
	p := preimage{h: sha256.New()}
	p.raw(marker)
	return p
}

// raw writes s exactly as given.
func (p preimage) raw(s string) { _, _ = p.h.Write([]byte(s)) }

// text writes s followed by the NUL separator.
func (p preimage) text(s string) { p.raw(s); p.raw("\x00") }

// basis writes each record's kind, key and digest in order.
func (p preimage) basis(records []BasisRecord) {
	for _, b := range records {
		p.text(b.Kind)
		p.text(b.Key)
		p.text(b.Digest)
	}
}

// sum is the fingerprint of everything written so far.
func (p preimage) sum() string {
	return definitionPrefix + hex.EncodeToString(p.h.Sum(nil))
}

// FileCall is the part of one File call that defines it (spec §11.4):
// canonical path, managed contents, managed mode, and the Basis already
// canonicalized (see ObserveBasis) so two equivalent Basis sets hash
// identically.
type FileCall struct {
	Path            string
	ContentsManaged bool
	Contents        []byte
	Mode            uint32
	Basis           []BasisRecord
}

// DefinitionFingerprint is the File's operation definition fingerprint:
// canonical path + managed contents digest + managed mode + sorted Basis
// descriptors.
func (c FileCall) DefinitionFingerprint() string {
	p := newPreimage(fileDefinitionMarker)
	p.text(c.Path)
	if c.ContentsManaged {
		sum := sha256.Sum256(c.Contents)
		p.raw(contentsManagedMarker)
		p.raw(string(sum[:]))
	} else {
		p.raw(contentsUnmanaged)
	}
	p.raw(fmt.Sprintf("mode:%d\x00", c.Mode))
	p.basis(c.Basis)
	return p.sum()
}

// ExecCall is the part of one Exec call that defines it (spec §11.4/§8.4):
// resolved executable, argv, directory, explicit Env, canonicalized Basis,
// and declared outputs resolved against Dir and sorted.
type ExecCall struct {
	ExecutablePath string
	Args           []string
	Dir            string
	Env            map[string]string
	Basis          []BasisRecord
	Outputs        []string
}

// definitionFingerprint is the Exec's operation definition fingerprint:
// resolved executable digest + argv + dir + sorted explicit Env + sorted
// Basis descriptors + sorted output paths.
func (c ExecCall) definitionFingerprint(executableDigest string) string {
	p := newPreimage(execDefinitionMarker)
	p.text(executableDigest)
	for _, a := range c.Args {
		p.text(a)
	}
	p.text(c.Dir)
	for _, k := range sortedEnvKeys(c.Env) {
		p.text(k + "=" + c.Env[k])
	}
	p.basis(c.Basis)
	for _, out := range c.Outputs {
		p.text(out)
	}
	return p.sum()
}

// sortedEnvKeys returns env's keys sorted, so two ExecSpecs with the same
// explicit Env entries always hash identically regardless of map iteration
// order (spec §11.4: "sorted explicit Env").
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// OpaqueTaskDefinitionFingerprint computes an opaque Task's own definition
// identity: the conservative application-fingerprint fallback ZYS-817
// Decisions (2026-09-23) requires when a Task's Define recorded no precise
// File/Exec/Patch operation of its own to prove freshness with. Scoped by
// the Task's own stable key so two different opaque Tasks never collide
// onto the same digest merely because the application fingerprint matches.
func OpaqueTaskDefinitionFingerprint(key, appFingerprint string) string {
	p := newPreimage(opaqueDefinitionMarker)
	p.text(key)
	p.text(appFingerprint)
	return p.sum()
}
