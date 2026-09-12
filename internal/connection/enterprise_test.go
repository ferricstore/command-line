package connection

import (
	"context"
	"testing"
	"time"

	"github.com/ferricstore/command-line/internal/platformapi"
	"github.com/ferricstore/command-line/internal/profile"
)

type enterpriseBrokerStub struct {
	controlURL   string
	token        string
	organization string
	cluster      string
	credential   platformapi.Credential
	err          error
}

func (b *enterpriseBrokerStub) Exchange(
	_ context.Context,
	controlURL, token, organization, cluster string,
	_ time.Duration,
) (platformapi.Credential, error) {
	b.controlURL = controlURL
	b.token = token
	b.organization = organization
	b.cluster = cluster
	return b.credential, b.err
}

func TestEnterpriseTokenProviderExchangesBeforeOpeningNativeClient(t *testing.T) {
	t.Parallel()

	broker := &enterpriseBrokerStub{credential: platformapi.Credential{
		Endpoint: "ferrics://store.example.com:6389",
		Username: "platform_cli_0123456789abcdef",
		Password: "temporary-native-secret",
	}}
	factory := &fakePasswordFactory{client: &fakeClient{}}
	provider := NewEnterpriseTokenProvider(profile.AuthMethodEnterpriseAPIToken, broker, factory)
	stored := profile.Profile{
		ControlURL:   "https://platform.example.com",
		Organization: "acme",
		Cluster:      "cluster-id",
		CACertFile:   "/etc/ferric/data-plane-ca.pem",
		Authentication: profile.Authentication{
			Method: profile.AuthMethodEnterpriseAPIToken,
		},
	}

	client, err := provider.Open(context.Background(), stored, "fsp_sa_control-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if broker.token != "fsp_sa_control-secret" || broker.cluster != "cluster-id" {
		t.Fatalf("broker input = %#v", broker)
	}
	if factory.profile.URL != broker.credential.Endpoint ||
		factory.profile.CACertFile != stored.CACertFile ||
		factory.profile.Authentication.Method != profile.AuthMethodPassword ||
		factory.profile.Authentication.Username != broker.credential.Username ||
		factory.password != broker.credential.Password {
		t.Fatalf("data-plane factory input = %#v/%q", factory.profile, factory.password)
	}
	if factory.password == broker.token {
		t.Fatal("Platform control token was forwarded to the data plane")
	}
}
