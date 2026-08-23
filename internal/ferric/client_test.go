package ferric

import "testing"

func TestNewClient(t *testing.T) {
	t.Parallel()

	client, err := NewClient("")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewClientAcceptsHTTPS(t *testing.T) {
	t.Parallel()

	client, err := NewClient("https://127.0.0.1:6388")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
