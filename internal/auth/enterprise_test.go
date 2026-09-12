package auth

import (
	"context"
	"testing"

	"github.com/ferricstore/command-line/internal/profile"
)

type enterpriseTokenValidatorStub struct {
	profile   profile.Profile
	token     string
	principal string
	err       error
}

func (v *enterpriseTokenValidatorStub) ValidateEnterpriseToken(
	_ context.Context,
	storedProfile profile.Profile,
	token string,
) (string, error) {
	v.profile = storedProfile
	v.token = token
	return v.principal, v.err
}

func TestEnterpriseTokenLoginPersistsControlTokenMetadataOnly(t *testing.T) {
	t.Parallel()

	validator := &enterpriseTokenValidatorStub{principal: "deploy-bot"}
	provider := NewEnterpriseTokenProvider(profile.AuthMethodEnterpriseAPIToken, validator)
	request := LoginRequest{
		ProfileName:  "production",
		Method:       profile.AuthMethodEnterpriseAPIToken,
		ControlURL:   "https://platform.example.com",
		Organization: "acme",
		Cluster:      "cluster-id",
		Secret:       "fsp_sa_control-secret",
	}

	result, err := provider.Login(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Principal != "deploy-bot" || result.Secret != request.Secret {
		t.Fatalf("result = %#v", result)
	}
	if result.Profile.URL != "" || result.Profile.ControlURL != request.ControlURL || result.Profile.Cluster != request.Cluster {
		t.Fatalf("profile = %#v", result.Profile)
	}
	if validator.token != request.Secret || validator.profile.Authentication.Method != request.Method {
		t.Fatalf("validator input = %#v/%q", validator.profile, validator.token)
	}
}
