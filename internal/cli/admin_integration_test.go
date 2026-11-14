//go:build integration

package cli

import (
	"os"
	"strings"
	"testing"
)

// TestIntegrationOSSNewerAdminCommands runs the cluster feature, sharded
// Pub/Sub, ACL and LATENCY helpers against a live FerricStore.
func TestIntegrationOSSNewerAdminCommands(t *testing.T) {
	if os.Getenv("FERRICSTORE_OSS_CLI_TEST") != "1" {
		t.Skip("OSS CLI integration is run by scripts/integration-login-oss.sh")
	}
	options := ossIntegrationOptions(t)
	expect := func(want string, args ...string) {
		t.Helper()
		if got := compactJSON(strings.TrimSpace(runIntegrationCLI(t, options, args...))); !strings.Contains(got, want) {
			t.Fatalf("ferric %s = %s; want it to contain %s", strings.Join(args, " "), got, want)
		}
	}

	runIntegrationCLI(t, options, "cluster", "features", "raise", "12", "--yes")
	expect("12", "cluster", "features")

	expect("0", "pubsub", "spublish", "cli:{shard}", "ready")
	runIntegrationCLI(t, options, "pubsub", "shard-channels")
	expect(`"cli:{shard}"`, "pubsub", "shard-subscribers", "cli:{shard}")

	expect(`"read"`, "acl", "cat")
	expect(`"get"`, "acl", "cat", "read")
	if password := strings.Trim(strings.TrimSpace(runIntegrationCLI(t, options, "acl", "genpass")), `"`); len(password) != 64 {
		t.Fatalf("acl genpass = %q; want 64 hex characters", password)
	}
	runIntegrationCLI(t, options, "acl", "log", "10")
	runIntegrationCLI(t, options, "acl", "log-reset", "--yes")
	expect("OK", "acl", "dryrun", "default", "get", "cli:key")

	runIntegrationCLI(t, options, "server", "latency", "latest")
	runIntegrationCLI(t, options, "server", "latency", "doctor")
	runIntegrationCLI(t, options, "server", "latency", "histogram")
	expect("0", "server", "latency", "reset", "--yes")
}
