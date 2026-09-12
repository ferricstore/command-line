package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	for _, name := range []string{
		"FERRIC_URL",
		"FERRIC_USERNAME",
		"FERRIC_PASSWORD",
		"FERRIC_PASSWORD_FILE",
		"FERRIC_CA_CERT_FILE",
		"FERRIC_CONTROL_URL",
		"FERRIC_ORGANIZATION",
		"FERRIC_CLUSTER",
		"FERRIC_API_TOKEN",
		"FERRIC_API_TOKEN_FILE",
		"FERRIC_PROFILE",
		"FERRIC_CONFIG_DIR",
	} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}
