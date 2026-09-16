package fingerprint

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrUnsupportedValueType is returned by Value's Fingerprint when v is not
// one of the v1 supported scalar types.
var ErrUnsupportedValueType = errors.New("fingerprint: unsupported Value type")

// ErrNonFiniteFloat is returned by Value's Fingerprint for a NaN or
// infinite float — spec §11.1 requires a finite IEEE-754 bit encoding.
var ErrNonFiniteFloat = errors.New("fingerprint: value is a non-finite float")

// Type tags distinguish otherwise-identical encoded byte strings across
// value kinds (e.g. bool true vs. the byte 0x01) so no two distinct typed
// values can collide on digest.
const (
	tagString byte = iota
	tagBool
	tagInt
	tagUint
	tagFloat
	tagTime
	tagBytes
)

// valueFingerprint implements Fingerprint for Value(name, v).
type valueFingerprint struct {
	name string
	v    any
}

// Value fingerprints a caller-supplied scalar under a stable, safe name
// (spec §11.1). Only the digest and name are ever persisted — never the raw
// value. Supported types: string, bool, any signed/unsigned integer, any
// finite float32/float64, time.Time (normalized to UTC nanoseconds), and
// []byte. Anything else is a typed error at Fingerprint time.
func Value(name string, v any) Fingerprint {
	return valueFingerprint{name: name, v: v}
}

func (f valueFingerprint) Fingerprint(_ context.Context) (FingerprintValue, error) {
	encoded, err := encodeValue(f.v)
	if err != nil {
		return FingerprintValue{}, fmt.Errorf("fingerprint value %q: %w", f.name, err)
	}
	return FingerprintValue{Kind: KindValue, Key: f.name, Digest: sum256(encoded)}, nil
}

// encodeValue produces v's canonical typed byte encoding: a one-byte type
// tag (so distinct types never collide) followed by a fixed-width or exact
// encoding of v itself.
func encodeValue(v any) ([]byte, error) {
	switch value := v.(type) {
	case string:
		return append([]byte{tagString}, value...), nil
	case bool:
		b := byte(0)
		if value {
			b = 1
		}
		return []byte{tagBool, b}, nil
	case int, int8, int16, int32, int64:
		return encodeInt64(tagInt, reflectInt64(value)), nil
	case uint, uint8, uint16, uint32, uint64:
		return encodeUint64(tagUint, reflectUint64(value)), nil
	case float32:
		return encodeFloat(float64(value))
	case float64:
		return encodeFloat(value)
	case time.Time:
		return encodeInt64(tagTime, value.UTC().UnixNano()), nil
	case []byte:
		return append([]byte{tagBytes}, value...), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedValueType, v)
	}
}

func encodeFloat(f float64) ([]byte, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, ErrNonFiniteFloat
	}
	return encodeUint64(tagFloat, math.Float64bits(f)), nil
}

func encodeInt64(tag byte, n int64) []byte {
	return encodeUint64(tag, uint64(n))
}

func encodeUint64(tag byte, n uint64) []byte {
	buf := make([]byte, 9)
	buf[0] = tag
	binary.BigEndian.PutUint64(buf[1:], n)
	return buf
}

// reflectInt64 widens any supported signed integer kind to int64 without a
// reflect import — a small exhaustive type switch on the concrete types
// encodeValue already matched.
func reflectInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

// reflectUint64 widens any supported unsigned integer kind to uint64.
func reflectUint64(v any) uint64 {
	switch n := v.(type) {
	case uint:
		return uint64(n)
	case uint8:
		return uint64(n)
	case uint16:
		return uint64(n)
	case uint32:
		return uint64(n)
	case uint64:
		return n
	default:
		return 0
	}
}
