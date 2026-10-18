package freshness

import "testing"

// Definition fingerprints are persisted in manifests, so a change to their
// preimage silently invalidates every prior record. These digests pin the
// preimage byte for byte.
func TestDefinitionFingerprintsKeepTheirPersistedPreimage(t *testing.T) {
	basis := []BasisRecord{
		{Kind: "app", Key: "application", Digest: "sha-a"},
		{Kind: "value", Key: "k", Digest: "d2"},
	}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			"file with managed contents and Basis",
			FileCall{Path: "/x", ContentsManaged: true, Contents: []byte("same"), Basis: basis}.DefinitionFingerprint(),
			"sha256:71ea38518ddc5c0af14d74c52e74cbd34016d444fca971b539c6d1fdf31f1857",
		},
		{
			"mode-only file",
			FileCall{Path: "/x", Mode: 0o755}.DefinitionFingerprint(),
			"sha256:9444e105e67c6c666eb3689f5b7cef88e58581220791ad7f375d84d0d822c9f0",
		},
		{
			"exec",
			ExecCall{
				Args: []string{"a", "b"}, Dir: "/d", Env: map[string]string{"B": "2", "A": "1"},
				Basis: basis, Outputs: []string{"o1", "o2"},
			}.definitionFingerprint("abc"),
			"sha256:7a99b4c0851389faf4c0bc987e4b571ff6749eb560f90f8cac85db7c6c1001f3",
		},
		{
			"opaque task",
			OpaqueTaskDefinitionFingerprint("t1", "sha256:aaaa"),
			"sha256:5097bff9846b8dfcc50030046539cb4664cea5e4478c63136106bbf545e88b4a",
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: fingerprint = %s, want %s", c.name, c.got, c.want)
		}
	}
}

// TestFileManifestAppBasisDriftForcesReconciliation proves spec §11.2: an explicit App()
// Basis entry ties an operation's freshness to the running application's own
// fingerprint, without requiring a manifest ApplicationRecord comparison — a
// changed App() digest is just an ordinary Basis drift.
func TestFileManifestAppBasisDriftForcesReconciliation(t *testing.T) {
	basisA := []BasisRecord{{Kind: "app", Key: "application", Digest: "sha-a"}}
	basisB := []BasisRecord{{Kind: "app", Key: "application", Digest: "sha-b"}}
	defA := FileCall{Path: "/x", ContentsManaged: true, Contents: []byte("same"), Basis: basisA}.DefinitionFingerprint()
	defB := FileCall{Path: "/x", ContentsManaged: true, Contents: []byte("same"), Basis: basisB}.DefinitionFingerprint()
	if defA == defB {
		t.Fatal("a changed App() Basis digest must change the operation definition fingerprint")
	}
}

// TestOpaqueTaskDefinitionFingerprintChangesWithAppFingerprint proves the
// fallback's own hash actually depends on the application fingerprint it
// falls back to, and on the Task it belongs to.
func TestOpaqueTaskDefinitionFingerprintChangesWithAppFingerprint(t *testing.T) {
	fpA := OpaqueTaskDefinitionFingerprint("t1", "sha256:aaaa")
	if fpA == OpaqueTaskDefinitionFingerprint("t1", "sha256:bbbb") {
		t.Fatal("a changed application fingerprint must change the opaque Task's own DefinitionFingerprint")
	}
	if fpA != OpaqueTaskDefinitionFingerprint("t1", "sha256:aaaa") {
		t.Fatal("the same key and application fingerprint must reproduce the same DefinitionFingerprint")
	}
	if fpA == OpaqueTaskDefinitionFingerprint("t2", "sha256:aaaa") {
		t.Fatal("two different Task keys must not collide onto the same opaque DefinitionFingerprint")
	}
}

// TestExecDefinitionIgnoresEnvMapOrder proves spec §11.4: explicit Env enters
// the definition sorted by name, so map iteration order never shows.
func TestExecDefinitionIgnoresEnvMapOrder(t *testing.T) {
	env := map[string]string{"A": "1", "B": "2", "C": "3", "D": "4"}
	want := ExecCall{Env: env}.definitionFingerprint("x")
	for range 20 {
		if got := (ExecCall{Env: env}).definitionFingerprint("x"); got != want {
			t.Fatalf("definition varied with map order: %s vs %s", got, want)
		}
	}
}
