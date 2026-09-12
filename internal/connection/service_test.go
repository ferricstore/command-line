package connection

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

type memoryProfileStore struct {
	values map[string]profile.Profile
}

func (s *memoryProfileStore) Put(_ context.Context, value profile.Profile) error {
	s.values[value.Name] = value
	return nil
}

func (s *memoryProfileStore) Get(_ context.Context, name string) (profile.Profile, error) {
	value, ok := s.values[name]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return value, nil
}

type memoryCredentialStore struct {
	values map[string]string
}

func (s *memoryCredentialStore) Put(_ context.Context, name, secret string) error {
	s.values[name] = secret
	return nil
}

func (s *memoryCredentialStore) Get(_ context.Context, name string) (string, error) {
	value, ok := s.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (s *memoryCredentialStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}

type fakeClient struct{}

func (*fakeClient) Ping(context.Context, ...string) (string, error) { return "PONG", nil }
func (*fakeClient) Close() error                                    { return nil }

type mockProvider struct {
	method  profile.AuthMethod
	profile profile.Profile
	secret  string
	client  Client
	err     error
}

func (p *mockProvider) Method() profile.AuthMethod {
	return p.method
}

func (p *mockProvider) Open(_ context.Context, storedProfile profile.Profile, secret string) (Client, error) {
	p.profile = storedProfile
	p.secret = secret
	return p.client, p.err
}

func TestOpenRoutesSavedStateToProvider(t *testing.T) {
	t.Parallel()

	stored := profile.Profile{
		Name:         "production",
		ControlURL:   "https://enterprise.example.com",
		Organization: "acme",
		Cluster:      "primary",
		Authentication: profile.Authentication{
			Method: profile.AuthMethodEnterpriseSSO,
		},
	}
	profiles := &memoryProfileStore{values: map[string]profile.Profile{"production": stored}}
	credentials := &memoryCredentialStore{values: map[string]string{"production": "renewable-session"}}
	provider := &mockProvider{method: profile.AuthMethodEnterpriseSSO, client: &fakeClient{}}
	service := NewService(profiles, credentials, provider)

	client, gotProfile, err := service.Open(context.Background(), " production ")
	if err != nil {
		t.Fatal(err)
	}
	if client != provider.client || gotProfile != stored || provider.profile != stored {
		t.Fatalf("Open() client/profile = %#v/%#v; provider profile = %#v", client, gotProfile, provider.profile)
	}
	if provider.secret != "renewable-session" {
		t.Fatalf("provider secret = %q", provider.secret)
	}
}

func TestOpenSupportsMockEnterpriseMethods(t *testing.T) {
	t.Parallel()

	for _, method := range []profile.AuthMethod{
		profile.AuthMethodEnterpriseSSO,
		profile.AuthMethodEnterpriseAPIToken,
	} {
		method := method
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()
			stored := profile.Profile{
				Name: "enterprise",
				Authentication: profile.Authentication{
					Method: method,
				},
			}
			provider := &mockProvider{method: method, client: &fakeClient{}}
			service := NewService(
				&memoryProfileStore{values: map[string]profile.Profile{"enterprise": stored}},
				&memoryCredentialStore{values: map[string]string{"enterprise": "exchange-me"}},
				provider,
			)
			if _, _, err := service.Open(context.Background(), "enterprise"); err != nil {
				t.Fatal(err)
			}
			if provider.secret != "exchange-me" {
				t.Fatal("provider did not receive the renewable Enterprise credential")
			}
		})
	}
}

func TestOpenEphemeralRoutesCredentialsWithoutStores(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{method: profile.AuthMethodPassword, client: &fakeClient{}}
	service := NewService(nil, nil, provider)
	wantProfile := profile.Profile{
		URL: "ferrics://store.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}

	client, err := service.OpenEphemeral(context.Background(), EphemeralCredentials{
		Profile: wantProfile,
		Secret:  "environment-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client != provider.client || provider.profile != wantProfile || provider.secret != "environment-secret" {
		t.Fatalf("OpenEphemeral() client/profile/secret = %#v/%#v/%q", client, provider.profile, provider.secret)
	}
}

func TestOpenEphemeralRejectsUnavailableAuthenticationMethod(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil)
	_, err := service.OpenEphemeral(context.Background(), EphemeralCredentials{
		Profile: profile.Profile{
			Authentication: profile.Authentication{Method: profile.AuthMethodEnterpriseAPIToken},
		},
		Secret: "machine-token",
	})
	if err == nil || !strings.Contains(err.Error(), "not available in this build") {
		t.Fatalf("OpenEphemeral() error = %v", err)
	}
}

func TestOpenRejectsUnavailableMethodBeforeReadingCredential(t *testing.T) {
	t.Parallel()

	stored := profile.Profile{
		Name: "enterprise",
		Authentication: profile.Authentication{
			Method: profile.AuthMethodEnterpriseSSO,
		},
	}
	credentials := &memoryCredentialStore{values: map[string]string{}}
	service := NewService(
		&memoryProfileStore{values: map[string]profile.Profile{"enterprise": stored}},
		credentials,
	)
	_, _, err := service.Open(context.Background(), "enterprise")
	if err == nil {
		t.Fatal("Open() error = nil, want unavailable method error")
	}
	if errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("Open() read credential before provider routing: %v", err)
	}
}

func TestOpenReturnsMissingProfileAndCredentialErrors(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{method: profile.AuthMethodPassword, client: &fakeClient{}}
	service := NewService(
		&memoryProfileStore{values: map[string]profile.Profile{}},
		&memoryCredentialStore{values: map[string]string{}},
		provider,
	)
	_, _, err := service.Open(context.Background(), "missing")
	if !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("missing profile error = %v", err)
	}

	service = NewService(
		&memoryProfileStore{values: map[string]profile.Profile{
			"missing-secret": {
				Name: "missing-secret",
				Authentication: profile.Authentication{
					Method: profile.AuthMethodPassword,
				},
			},
		}},
		&memoryCredentialStore{values: map[string]string{}},
		provider,
	)
	_, _, err = service.Open(context.Background(), "missing-secret")
	if !errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("missing credential error = %v", err)
	}
}

type fakePasswordFactory struct {
	url      string
	username string
	password string
	client   Client
	err      error
}

func (f *fakePasswordFactory) NewPasswordClient(rawURL, username, password string) (Client, error) {
	f.url = rawURL
	f.username = username
	f.password = password
	return f.client, f.err
}

func TestPasswordProviderConstructsAuthenticatedClient(t *testing.T) {
	t.Parallel()

	factory := &fakePasswordFactory{client: &fakeClient{}}
	provider := NewPasswordProvider(factory)
	stored := profile.Profile{
		Name: "oss",
		URL:  "ferric://127.0.0.1:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	client, err := provider.Open(context.Background(), stored, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if client != factory.client {
		t.Fatal("Open() returned the wrong client")
	}
	if factory.url != stored.URL || factory.username != "operator" || factory.password != "secret" {
		t.Fatalf("factory input = %q/%q/%q", factory.url, factory.username, factory.password)
	}
}

func TestPasswordProviderRejectsCredentialsInSavedProfileURL(t *testing.T) {
	t.Parallel()

	factory := &fakePasswordFactory{client: &fakeClient{}}
	provider := NewPasswordProvider(factory)
	stored := profile.Profile{
		Name: "unsafe",
		URL:  "ferric://embedded:secret@store.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}

	client, err := provider.Open(context.Background(), stored, "keyring-secret")
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("Open() client/error = %#v/%v, want credential-bearing URL rejection", client, err)
	}
	if factory.url != "" || factory.username != "" || factory.password != "" {
		t.Fatalf("unsafe profile reached password client factory: %q/%q/%q", factory.url, factory.username, factory.password)
	}
}

func TestServiceLoadsCredentialReferenceFromProfile(t *testing.T) {
	t.Parallel()

	const reference = "production-credential-generation"
	storedProfile := profile.Profile{
		Name: "production",
		URL:  "ferric://store.example.com:6388",
		Authentication: profile.Authentication{
			Method:        profile.AuthMethodPassword,
			Username:      "operator",
			CredentialRef: reference,
		},
	}
	profiles := &memoryProfileStore{values: map[string]profile.Profile{"production": storedProfile}}
	credentials := &memoryCredentialStore{values: map[string]string{reference: "referenced-secret"}}
	provider := &mockProvider{method: profile.AuthMethodPassword, client: &fakeClient{}}
	service := NewService(profiles, credentials, provider)

	client, gotProfile, err := service.Open(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if gotProfile != storedProfile || provider.secret != "referenced-secret" {
		t.Fatalf("Open() profile/secret = %#v/%q", gotProfile, provider.secret)
	}
}
