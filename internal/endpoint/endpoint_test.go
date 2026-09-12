package endpoint

import (
	"strings"
	"testing"
)

func TestValidateRejectsCredentialBearingEndpointWithoutEchoingSecret(t *testing.T) {
	t.Parallel()

	_, err := Validate("ferric://operator:very-secret@store.example.com:6388", "FerricStore URL")
	if err == nil {
		t.Fatal("Validate() succeeded")
	}
	if strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("Validate() leaked secret: %v", err)
	}
}

func TestDisplayStripsAllNonEndpointURLComponents(t *testing.T) {
	t.Parallel()

	got := Display("ferrics://operator:very-secret@store.example.com:6389?timeout=5s#token")
	if got != "ferrics://store.example.com:6389" {
		t.Fatalf("Display() = %q", got)
	}
}
