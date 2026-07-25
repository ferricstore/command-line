package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	values map[string]string
	err    error
}

func (f *fakeKeyring) Set(service, user, password string) error {
	if f.err != nil {
		return f.err
	}
	f.values[service+"/"+user] = password
	return nil
}

func (f *fakeKeyring) Get(service, user string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	if f.err != nil {
		return f.err
	}
	key := service + "/" + user
	if _, ok := f.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, key)
	return nil
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	t.Parallel()

	backend := &fakeKeyring{values: make(map[string]string)}
	store := &KeyringStore{backend: backend}
	ctx := context.Background()
	if err := store.Put(ctx, "production", "secret"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "production")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Fatalf("Get() = %q, want secret", got)
	}
	if err := store.Delete(ctx, "production"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "production"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestKeyringStoreMapsBackendErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("backend unavailable")
	store := &KeyringStore{backend: &fakeKeyring{values: make(map[string]string), err: want}}
	if err := store.Put(context.Background(), "production", "secret"); !errors.Is(err, want) {
		t.Fatalf("Put() error = %v, want backend error", err)
	}
}
