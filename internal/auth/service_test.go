package auth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/credential"
	"github.com/ferricstore/command-line/internal/profile"
)

type coordinatedProfileStore struct {
	mu             sync.Mutex
	mutation       chan struct{}
	values         map[string]profile.Profile
	blockURL       string
	putBlocked     chan struct{}
	releaseBlocked chan struct{}
}

func newCoordinatedProfileStore(blockURL string) *coordinatedProfileStore {
	mutation := make(chan struct{}, 1)
	mutation <- struct{}{}
	return &coordinatedProfileStore{
		mutation:       mutation,
		values:         make(map[string]profile.Profile),
		blockURL:       blockURL,
		putBlocked:     make(chan struct{}),
		releaseBlocked: make(chan struct{}),
	}
}

func (s *coordinatedProfileStore) Put(ctx context.Context, value profile.Profile) error {
	if value.URL == s.blockURL {
		close(s.putBlocked)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.releaseBlocked:
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[value.Name] = value
	return nil
}

func (s *coordinatedProfileStore) Get(_ context.Context, name string) (profile.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[name]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return value, nil
}

func (s *coordinatedProfileStore) WithCredentialMutation(ctx context.Context, operation func(context.Context) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.mutation:
	}
	defer func() { s.mutation <- struct{}{} }()
	return operation(ctx)
}

type concurrentCredentialStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *concurrentCredentialStore) Put(_ context.Context, name, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = secret
	return nil
}

func (s *concurrentCredentialStore) Get(_ context.Context, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (s *concurrentCredentialStore) Delete(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[name]; !ok {
		return credential.ErrNotFound
	}
	delete(s.values, name)
	return nil
}

type memoryProfileStore struct {
	values map[string]profile.Profile
	putErr error
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{values: make(map[string]profile.Profile)}
}

func (s *memoryProfileStore) Put(_ context.Context, value profile.Profile) error {
	if s.putErr != nil {
		return s.putErr
	}
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
	putErr error
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{values: make(map[string]string)}
}

func (s *memoryCredentialStore) Put(_ context.Context, name, secret string) error {
	if s.putErr != nil {
		return s.putErr
	}
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
	if _, ok := s.values[name]; !ok {
		return credential.ErrNotFound
	}
	delete(s.values, name)
	return nil
}

type fakeValidator struct {
	err      error
	profile  profile.Profile
	password string
}

func (v *fakeValidator) ValidatePassword(_ context.Context, storedProfile profile.Profile, password string) error {
	v.profile = storedProfile
	v.password = password
	return v.err
}

type mockProvider struct {
	method  profile.AuthMethod
	request LoginRequest
	result  ProviderResult
	err     error
}

type rollbackCredentialStore struct {
	values      map[string]string
	putCalls    int
	rollbackErr error
}

func (s *rollbackCredentialStore) Put(ctx context.Context, name, secret string) error {
	s.putCalls++
	if s.putCalls > 1 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.rollbackErr != nil {
			return s.rollbackErr
		}
	}
	s.values[name] = secret
	return nil
}

func (s *rollbackCredentialStore) Get(_ context.Context, name string) (string, error) {
	value, ok := s.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (s *rollbackCredentialStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name != "production" && s.rollbackErr != nil {
		return s.rollbackErr
	}
	delete(s.values, name)
	return nil
}

type cancelingProfileStore struct {
	cancel func()
	err    error
}

type panickingProfileStore struct {
	values map[string]profile.Profile
}

func (s *panickingProfileStore) Put(_ context.Context, value profile.Profile) error {
	if _, exists := s.values[value.Name]; exists {
		panic("simulated process interruption")
	}
	s.values[value.Name] = value
	return nil
}

func (s *panickingProfileStore) Get(_ context.Context, name string) (profile.Profile, error) {
	value, ok := s.values[name]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return value, nil
}

func (s cancelingProfileStore) Put(_ context.Context, _ profile.Profile) error {
	s.cancel()
	return s.err
}

func (cancelingProfileStore) Get(_ context.Context, _ string) (profile.Profile, error) {
	return profile.Profile{}, profile.ErrNotFound
}

func (p *mockProvider) Method() profile.AuthMethod {
	return p.method
}

func (p *mockProvider) Login(_ context.Context, request LoginRequest) (ProviderResult, error) {
	p.request = request
	return p.result, p.err
}

func TestPasswordLoginValidatesAndPersists(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	validator := &fakeValidator{}
	service := NewService(profiles, credentials, NewPasswordProvider(validator))
	request := LoginRequest{
		ProfileName: "production",
		Method:      profile.AuthMethodPassword,
		URL:         "ferric://store.example.com:6388",
		Username:    "operator",
		Secret:      "password",
		Store:       true,
	}
	result, err := service.Login(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stored || result.Principal != request.Username {
		t.Fatalf("Login() = %#v", result)
	}
	if validator.profile.URL != request.URL || validator.profile.Authentication.Username != request.Username || validator.password != request.Secret {
		t.Fatalf("validator input = %#v/%q", validator.profile, validator.password)
	}
	if got := credentials.values[result.Profile.CredentialReference()]; got != request.Secret {
		t.Fatalf("stored credential = %q, want password", got)
	}
	if got := profiles.values[request.ProfileName]; got.Authentication.Method != profile.AuthMethodPassword {
		t.Fatalf("stored profile = %#v", got)
	}
}

func TestPasswordLoginNormalizesCACertificatePathBeforeValidationAndPersistence(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	validator := &fakeValidator{}
	service := NewService(profiles, credentials, NewPasswordProvider(validator))
	relativeCAFile := filepath.Join("testdata", "private-ca.pem")
	wantCAFile, err := filepath.Abs(relativeCAFile)
	if err != nil {
		t.Fatal(err)
	}
	request := LoginRequest{
		ProfileName: "production",
		URL:         "https://store.example.com",
		CACertFile:  relativeCAFile,
		Username:    "operator",
		Secret:      "password",
		Store:       true,
	}
	result, err := service.Login(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if validator.profile.CACertFile != wantCAFile {
		t.Fatalf("validator CA certificate = %q, want %q", validator.profile.CACertFile, wantCAFile)
	}
	if result.Profile.CACertFile != wantCAFile {
		t.Fatalf("result CA certificate = %q, want %q", result.Profile.CACertFile, wantCAFile)
	}
	if stored := profiles.values[request.ProfileName]; stored.CACertFile != wantCAFile {
		t.Fatalf("stored CA certificate = %q, want %q", stored.CACertFile, wantCAFile)
	}
}

func TestPasswordLoginFailureStoresNothing(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	want := errors.New("invalid username-password pair")
	service := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{err: want}))
	_, err := service.Login(context.Background(), LoginRequest{
		ProfileName: "production",
		URL:         "ferric://store.example.com:6388",
		Username:    "operator",
		Secret:      "wrong",
		Store:       true,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Login() error = %v, want authentication error", err)
	}
	if len(profiles.values) != 0 || len(credentials.values) != 0 {
		t.Fatalf("failed login persisted state: profiles=%v credentials=%v", profiles.values, credentials.values)
	}
}

func TestPasswordLoginRejectsCredentialsInURL(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	validator := &fakeValidator{}
	service := NewService(profiles, credentials, NewPasswordProvider(validator))
	_, err := service.Login(context.Background(), LoginRequest{
		ProfileName: "production",
		URL:         "ferric://operator:embedded-secret@store.example.com:6388",
		Username:    "operator",
		Secret:      "prompt-secret",
		Store:       true,
	})
	if err == nil {
		t.Fatal("Login() accepted credentials embedded in the URL")
	}
	if strings.Contains(err.Error(), "embedded-secret") {
		t.Fatal("rejected URL error exposed embedded credentials")
	}
	if validator.profile.URL != "" {
		t.Fatal("credential-bearing URL reached the SDK validator")
	}
	if len(profiles.values) != 0 || len(credentials.values) != 0 {
		t.Fatal("rejected URL persisted login state")
	}
}

func TestEnterpriseProvidersAreMockable(t *testing.T) {
	t.Parallel()

	methods := []profile.AuthMethod{
		profile.AuthMethodEnterpriseSSO,
		profile.AuthMethodEnterpriseAPIToken,
	}
	for _, method := range methods {
		method := method
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()
			profiles := newMemoryProfileStore()
			credentials := newMemoryCredentialStore()
			provider := &mockProvider{
				method: method,
				result: ProviderResult{
					Profile: profile.Profile{
						Name:         "enterprise",
						ControlURL:   "https://enterprise.example.com",
						Organization: "acme",
						Cluster:      "production",
						Authentication: profile.Authentication{
							Method: method,
						},
					},
					Principal: "principal-123",
					Secret:    "renewable-enterprise-credential",
				},
			}
			service := NewService(profiles, credentials, provider)
			result, err := service.Login(context.Background(), LoginRequest{
				ProfileName: "enterprise",
				Method:      method,
				Store:       true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Stored || provider.request.Method != method {
				t.Fatalf("Login() = %#v, request = %#v", result, provider.request)
			}
			if credentials.values[result.Profile.CredentialReference()] != provider.result.Secret {
				t.Fatal("mock Enterprise credential was not persisted through the provider boundary")
			}
		})
	}
}

func TestLoginRejectsProviderAuthenticationMethodMismatch(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	credentials := newMemoryCredentialStore()
	provider := &mockProvider{
		method: profile.AuthMethodEnterpriseSSO,
		result: ProviderResult{
			Profile: profile.Profile{
				Name: "enterprise",
				URL:  "ferric://store.example.com:6388",
				Authentication: profile.Authentication{
					Method: profile.AuthMethodPassword,
				},
			},
			Principal: "principal-123",
			Secret:    "renewable-enterprise-credential",
		},
	}
	service := NewService(profiles, credentials, provider)

	_, err := service.Login(context.Background(), LoginRequest{
		ProfileName: "enterprise",
		Method:      profile.AuthMethodEnterpriseSSO,
		Store:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "mismatched authentication method") {
		t.Fatalf("Login() error = %v, want provider authentication method mismatch", err)
	}
	if len(profiles.values) != 0 || len(credentials.values) != 0 {
		t.Fatalf("mismatched provider persisted state: profiles=%v credentials=%v", profiles.values, credentials.values)
	}
}

func TestProfileFailureRestoresPreviousCredential(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	profiles.putErr = fmt.Errorf("profile write failed")
	credentials := newMemoryCredentialStore()
	credentials.values["production"] = "previous"
	service := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{}))
	_, err := service.Login(context.Background(), LoginRequest{
		ProfileName: "production",
		URL:         "ferric://store.example.com:6388",
		Username:    "operator",
		Secret:      "replacement",
		Store:       true,
	})
	if !errors.Is(err, profiles.putErr) {
		t.Fatalf("Login() error = %v, want profile error", err)
	}
	if got := credentials.values["production"]; got != "previous" {
		t.Fatalf("credential after rollback = %q, want previous", got)
	}
}

func TestProfileFailureReportsCredentialRollbackFailure(t *testing.T) {
	t.Parallel()

	profileErr := errors.New("profile write failed")
	rollbackErr := errors.New("credential rollback failed")
	profiles := newMemoryProfileStore()
	profiles.putErr = profileErr
	credentials := &rollbackCredentialStore{
		values:      map[string]string{"production": "previous"},
		rollbackErr: rollbackErr,
	}
	service := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{}))
	_, err := service.Login(context.Background(), LoginRequest{
		ProfileName: "production",
		URL:         "ferric://store.example.com:6388",
		Username:    "operator",
		Secret:      "replacement",
		Store:       true,
	})
	if !errors.Is(err, profileErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("Login() error = %v, want profile and rollback errors", err)
	}
}

func TestProfileFailureRollsBackWithCanceledRequestContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	profileErr := errors.New("profile write failed")
	credentials := &rollbackCredentialStore{values: map[string]string{"production": "previous"}}
	service := NewService(
		cancelingProfileStore{cancel: cancel, err: profileErr},
		credentials,
		NewPasswordProvider(&fakeValidator{}),
	)
	_, err := service.Login(ctx, LoginRequest{
		ProfileName: "production",
		URL:         "ferric://store.example.com:6388",
		Username:    "operator",
		Secret:      "replacement",
		Store:       true,
	})
	if !errors.Is(err, profileErr) {
		t.Fatalf("Login() error = %v, want profile error", err)
	}
	if got := credentials.values["production"]; got != "previous" {
		t.Fatalf("credential after canceled-context rollback = %q, want previous", got)
	}
}

func TestLogoutRemovesOnlyLocalCredentialAndIsIdempotent(t *testing.T) {
	t.Parallel()

	profiles := newMemoryProfileStore()
	profiles.values["production"] = profile.Profile{Name: "production"}
	credentials := newMemoryCredentialStore()
	credentials.values["production"] = "secret"
	service := NewService(profiles, credentials)

	result, err := service.Logout(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Removed {
		t.Fatal("Logout() did not report credential removal")
	}
	if _, ok := credentials.values["production"]; ok {
		t.Fatal("Logout() retained the credential")
	}
	if _, ok := profiles.values["production"]; !ok {
		t.Fatal("Logout() removed profile metadata")
	}

	result, err = service.Logout(context.Background(), "production")
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed {
		t.Fatal("second Logout() reported a removal")
	}
}

func TestLogoutRequiresProfilePersistenceForGeneratedCredentialReference(t *testing.T) {
	t.Parallel()

	credentials := newMemoryCredentialStore()
	credentials.values["production"] = "legacy-secret"
	service := NewService(nil, credentials)

	_, err := service.Logout(context.Background(), "production")
	if err == nil || !strings.Contains(err.Error(), "profile persistence") {
		t.Fatalf("Logout() error = %v", err)
	}
	if credentials.values["production"] != "legacy-secret" {
		t.Fatal("Logout() mutated credentials without profile metadata")
	}
}

func TestConcurrentLoginsCannotPairProfileWithAnotherLoginSecret(t *testing.T) {
	t.Parallel()

	const (
		profileName = "production"
		urlA        = "ferric://cluster-a.example.com:6388"
		urlB        = "ferric://cluster-b.example.com:6388"
		secretA     = "secret-a"
		secretB     = "secret-b"
	)
	profiles := newCoordinatedProfileStore(urlA)
	credentials := &concurrentCredentialStore{values: make(map[string]string)}
	serviceA := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{}))
	serviceB := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{}))

	login := func(service *Service, rawURL, secret string) <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := service.Login(context.Background(), LoginRequest{
				ProfileName: profileName,
				URL:         rawURL,
				Username:    "operator",
				Secret:      secret,
				Store:       true,
			})
			done <- err
		}()
		return done
	}

	doneA := login(serviceA, urlA, secretA)
	select {
	case <-profiles.putBlocked:
	case <-time.After(time.Second):
		t.Fatal("first login did not reach the blocked profile write")
	}
	doneB := login(serviceB, urlB, secretB)
	var (
		errB       error
		bCompleted bool
	)
	select {
	case errB = <-doneB:
		bCompleted = true
		// Without transaction coordination, the second login can finish while
		// the first is paused between its credential and profile writes.
	case <-time.After(100 * time.Millisecond):
	}
	close(profiles.releaseBlocked)
	if err := <-doneA; err != nil {
		t.Fatal(err)
	}
	if !bCompleted {
		select {
		case errB = <-doneB:
		case <-time.After(time.Second):
			t.Fatal("second login did not complete")
		}
	}
	if errB != nil {
		t.Fatal(errB)
	}

	storedProfile, err := profiles.Get(context.Background(), profileName)
	if err != nil {
		t.Fatal(err)
	}
	storedSecret, err := credentials.Get(context.Background(), storedProfile.CredentialReference())
	if err != nil {
		t.Fatal(err)
	}
	wantSecret := secretA
	if storedProfile.URL == urlB {
		wantSecret = secretB
	}
	if storedSecret != wantSecret {
		t.Fatalf("stored profile %q is paired with secret %q, want %q", storedProfile.URL, storedSecret, wantSecret)
	}
}

func TestInterruptedLoginCannotReplaceCredentialUsedByPreviousProfile(t *testing.T) {
	t.Parallel()

	const profileName = "production"
	oldProfile := profile.Profile{
		Name: profileName,
		URL:  "ferric://cluster-a.example.com:6388",
		Authentication: profile.Authentication{
			Method:   profile.AuthMethodPassword,
			Username: "operator",
		},
	}
	profiles := &panickingProfileStore{values: map[string]profile.Profile{profileName: oldProfile}}
	credentials := newMemoryCredentialStore()
	credentials.values[profileName] = "secret-a"
	service := NewService(profiles, credentials, NewPasswordProvider(&fakeValidator{}))

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Login() did not simulate an interrupted profile write")
			}
		}()
		_, _ = service.Login(context.Background(), LoginRequest{
			ProfileName: profileName,
			URL:         "ferric://cluster-b.example.com:6388",
			Username:    "operator",
			Secret:      "secret-b",
			Store:       true,
		})
	}()

	storedProfile, err := profiles.Get(context.Background(), profileName)
	if err != nil {
		t.Fatal(err)
	}
	if storedProfile.URL != oldProfile.URL {
		t.Fatalf("stored profile URL = %q, want previous %q", storedProfile.URL, oldProfile.URL)
	}
	if secret := credentials.values[profileName]; secret != "secret-a" {
		t.Fatalf("credential used by previous profile = %q, want unchanged", secret)
	}
}
