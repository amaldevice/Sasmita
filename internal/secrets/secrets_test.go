package secrets_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/secrets"
)

// fakeRunner observes how the Keychain implementation invokes `security`
// without touching the real macOS Keychain.
type fakeRunner struct {
	args  [][]string
	stdin []string
	out   []byte
	err   error
}

func (f *fakeRunner) Run(_ context.Context, args []string, stdin string) ([]byte, error) {
	f.args = append(f.args, args)
	f.stdin = append(f.stdin, stdin)
	return f.out, f.err
}

func TestKeychainPutKeepsSecretOutOfArguments(t *testing.T) {
	runner := &fakeRunner{}
	store := secrets.Keychain{Service: "Sasmita", Runner: runner}
	if err := store.Put(t.Context(), "profile-1", "super-secret-key"); err != nil {
		t.Fatal(err)
	}
	if len(runner.args) != 1 {
		t.Fatalf("invocations = %d", len(runner.args))
	}
	if strings.Contains(fmt.Sprint(runner.args[0]), "super-secret-key") {
		t.Fatalf("secret leaked into argv: %v", runner.args[0])
	}
	if runner.stdin[0] != "super-secret-key" {
		t.Fatalf("secret was not supplied on stdin: %q", runner.stdin[0])
	}
}

func TestKeychainGetReadsSecretFromStdout(t *testing.T) {
	runner := &fakeRunner{out: []byte("super-secret-key\n")}
	store := secrets.Keychain{Service: "Sasmita", Runner: runner}
	got, err := store.Get(t.Context(), "profile-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "super-secret-key" {
		t.Fatalf("Get = %q", got)
	}
}

func TestKeychainGetMapsMissingItemToNotFound(t *testing.T) {
	runner := &fakeRunner{err: errors.New("SecKeychainSearchCopyNext: The specified item could not be found in the keychain.")}
	store := secrets.Keychain{Service: "Sasmita", Runner: runner}
	_, err := store.Get(t.Context(), "missing")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// A fake Store proves the port is swappable; appservice tests use this shape.
type fakeStore struct{ values map[string]string }

func (f *fakeStore) Put(_ context.Context, ref, value string) error {
	f.values[ref] = value
	return nil
}
func (f *fakeStore) Get(_ context.Context, ref string) (string, error) {
	v, ok := f.values[ref]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}
func (f *fakeStore) Delete(_ context.Context, ref string) error {
	delete(f.values, ref)
	return nil
}

func TestFakeSecretsStoreRoundTrip(t *testing.T) {
	var store secrets.Store = &fakeStore{values: map[string]string{}}
	if err := store.Put(t.Context(), "ref", "key"); err != nil {
		t.Fatal(err)
	}
	v, err := store.Get(t.Context(), "ref")
	if err != nil || v != "key" {
		t.Fatalf("Get = %q (err %v)", v, err)
	}
	if err := store.Delete(t.Context(), "ref"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), "ref"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("error after delete = %v", err)
	}
}
