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

func TestNewClientRejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	client, err := NewClient("https://127.0.0.1:6388")
	if err == nil {
		if client != nil {
			_ = client.Close()
		}
		t.Fatal("NewClient() error = nil, want unsupported scheme error")
	}
}
