package fingerprint

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestValueStoresDigestAndSafeNameNotRawValue(t *testing.T) {
	fp, err := Value("password", "super-secret").Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fp.Kind != KindValue {
		t.Fatalf("Kind = %q", fp.Kind)
	}
	if fp.Key != "password" {
		t.Fatalf("Key = %q, want the safe name only", fp.Key)
	}
	var zero [32]byte
	if fp.Digest == zero {
		t.Fatal("Digest was never populated")
	}
}

func TestValueSameInputSameDigest(t *testing.T) {
	a, err := Value("n", int64(42)).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Value("n", int64(42)).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatal("identical Value inputs produced different digests")
	}
}

func TestValueDistinctTypesDoNotCollide(t *testing.T) {
	str, err := Value("n", "1").Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	num, err := Value("n", int64(1)).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if str.Digest == num.Digest {
		t.Fatal(`Value("n","1") collided with Value("n", int64(1))`)
	}
}

func TestValueTimeNormalizedToUTCNanoseconds(t *testing.T) {
	utc := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	inOtherZone := utc.In(time.FixedZone("X", 3600))
	a, err := Value("t", utc).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Value("t", inOtherZone).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatal("the same instant in two zones produced different digests")
	}
}

func TestValueNonFiniteFloatIsTypedError(t *testing.T) {
	_, err := Value("f", math.NaN()).Fingerprint(context.Background())
	if !errors.Is(err, ErrNonFiniteFloat) {
		t.Fatalf("err = %v, want ErrNonFiniteFloat", err)
	}
	_, err = Value("f", math.Inf(1)).Fingerprint(context.Background())
	if !errors.Is(err, ErrNonFiniteFloat) {
		t.Fatalf("err = %v, want ErrNonFiniteFloat", err)
	}
}

func TestValueUnsupportedTypeIsTypedError(t *testing.T) {
	_, err := Value("v", struct{ X int }{1}).Fingerprint(context.Background())
	if !errors.Is(err, ErrUnsupportedValueType) {
		t.Fatalf("err = %v, want ErrUnsupportedValueType", err)
	}
}
