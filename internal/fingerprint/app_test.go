package fingerprint

import (
	"context"
	"errors"
	"os"
	"testing"
)

type fakeAppEnvironment struct {
	exePath      string
	exeErr       error
	fileContents map[string][]byte
	buildID      string
	buildOK      bool
}

func (f fakeAppEnvironment) Executable() (string, error) { return f.exePath, f.exeErr }
func (f fakeAppEnvironment) ReadFile(path string) ([]byte, error) {
	b, ok := f.fileContents[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}
func (f fakeAppEnvironment) ReadBuildInfo() (string, bool) { return f.buildID, f.buildOK }

func TestAppFingerprintsExecutableBytesWhenReadable(t *testing.T) {
	env := fakeAppEnvironment{
		exePath:      "/bin/app",
		fileContents: map[string][]byte{"/bin/app": []byte("binary-v1")},
	}
	var fp FingerprintValue
	var err error
	withAppEnvironment(env, func() {
		fp, err = App().Fingerprint(context.Background())
	})
	if err != nil {
		t.Fatal(err)
	}
	if fp.Kind != KindApp || fp.Key != "application" {
		t.Fatalf("Kind=%q Key=%q", fp.Kind, fp.Key)
	}

	changed := env
	changed.fileContents = map[string][]byte{"/bin/app": []byte("binary-v2")}
	var fp2 FingerprintValue
	withAppEnvironment(changed, func() {
		fp2, err = App().Fingerprint(context.Background())
	})
	if err != nil {
		t.Fatal(err)
	}
	if fp.Digest == fp2.Digest {
		t.Fatal("changed executable bytes produced the same digest")
	}
}

func TestAppFallsBackToBuildIDWhenExecutableUnreadable(t *testing.T) {
	env := fakeAppEnvironment{
		exeErr:  os.ErrNotExist,
		buildID: "example.com/mod@deadbeef",
		buildOK: true,
	}
	var fp FingerprintValue
	var err error
	withAppEnvironment(env, func() {
		fp, err = App().Fingerprint(context.Background())
	})
	if err != nil {
		t.Fatal(err)
	}
	if fp.Kind != KindApp || fp.Key != "application" {
		t.Fatalf("Kind=%q Key=%q", fp.Kind, fp.Key)
	}
}

func TestAppUnavailableIsTypedError(t *testing.T) {
	env := fakeAppEnvironment{exeErr: os.ErrNotExist, buildOK: false}
	var err error
	withAppEnvironment(env, func() {
		_, err = App().Fingerprint(context.Background())
	})
	if !errors.Is(err, ErrAppFingerprintUnavailable) {
		t.Fatalf("err = %v, want ErrAppFingerprintUnavailable", err)
	}
}
