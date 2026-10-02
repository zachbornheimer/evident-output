// Package download fetches a URL into a writer and proves the bytes match
// an integrity string. It never touches a destination: callers stage the
// bytes and publish them only after Fetch returns nil.
package download

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
)

// Failure classes. The engine re-exports these as its public errors.
var (
	// ErrIntegrity is bytes that differ from the Integrity, or an
	// Integrity that is empty or cannot be parsed.
	ErrIntegrity = errors.New("evo: Download integrity mismatch")
	// ErrFailed is a fetch that did not complete with a 2xx response.
	ErrFailed = errors.New("evo: Download failed")
	// ErrURLMissing is a Download with an empty URL.
	ErrURLMissing = errors.New("evo: Download URL is required")
)

type algorithm struct {
	name string
	size int
	new  func() hash.Hash
	rank int
}

var (
	sha512Alg = algorithm{"sha512", sha512.Size, sha512.New, 3}
	sha384Alg = algorithm{"sha384", sha512.Size384, sha512.New384, 2}
	sha256Alg = algorithm{"sha256", sha256.Size, sha256.New, 1}
	// sha1Alg exists only for the plain-hex Integrity form registries still publish.
	sha1Alg = algorithm{"sha1", sha1.Size, sha1.New, 0}
)

var sriAlgorithms = []algorithm{sha512Alg, sha384Alg, sha256Alg}

// Expected is a parsed Integrity: one algorithm and the digest to match.
type Expected struct {
	alg    algorithm
	digest []byte
}

// Algorithm names the hash that Expected checks, such as "sha512".
func (e Expected) Algorithm() string { return e.alg.name }

// Parse reads an SRI string ("sha512-<b64>", several tokens allowed, the
// strongest algorithm wins) or a plain hex sha256 or sha1 digest.
func Parse(integrity string) (Expected, error) {
	tokens := strings.Fields(integrity)
	if len(tokens) == 0 {
		return Expected{}, fmt.Errorf("%w: Integrity is empty", ErrIntegrity)
	}
	if len(tokens) == 1 && !strings.Contains(tokens[0], "-") {
		return parseHex(tokens[0])
	}
	var best *Expected
	for _, token := range tokens {
		e, known, err := parseSRIToken(token)
		if err != nil {
			return Expected{}, err
		}
		if known && (best == nil || e.alg.rank > best.alg.rank) {
			best = &e
		}
	}
	if best == nil {
		return Expected{}, fmt.Errorf("%w: no supported algorithm (want sha256, sha384, or sha512)", ErrIntegrity)
	}
	return *best, nil
}

// parseSRIToken parses one "alg-b64[?opts]" token. known is false for an
// algorithm SRI does not allow, which is skipped, not fatal, per W3C.
func parseSRIToken(token string) (e Expected, known bool, err error) {
	algName, rest, _ := strings.Cut(token, "-")
	rest, _, _ = strings.Cut(rest, "?")
	for _, alg := range sriAlgorithms {
		if !strings.EqualFold(alg.name, algName) {
			continue
		}
		digest, err := decodeBase64(rest)
		if err != nil || len(digest) != alg.size {
			return Expected{}, true, fmt.Errorf("%w: malformed %s digest", ErrIntegrity, alg.name)
		}
		return Expected{alg: alg, digest: digest}, true, nil
	}
	return Expected{}, false, nil
}

func decodeBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func parseHex(s string) (Expected, error) {
	for _, alg := range []algorithm{sha256Alg, sha1Alg} {
		if len(s) != alg.size*2 {
			continue
		}
		digest, err := hex.DecodeString(s)
		if err != nil {
			break
		}
		return Expected{alg: alg, digest: digest}, nil
	}
	return Expected{}, fmt.Errorf("%w: want a sha256 or sha1 hex digest or an SRI string", ErrIntegrity)
}

// Hasher starts a running digest of the algorithm Expected checks.
func (e Expected) Hasher() hash.Hash { return e.alg.new() }

// Match reports whether sum is the digest Expected demands.
func (e Expected) Match(sum []byte) bool {
	return subtle.ConstantTimeCompare(sum, e.digest) == 1
}

// Matches reads r to its end and reports whether its bytes match.
func (e Expected) Matches(r io.Reader) (bool, error) {
	h := e.Hasher()
	if _, err := io.Copy(h, r); err != nil {
		return false, err
	}
	return e.Match(h.Sum(nil)), nil
}
