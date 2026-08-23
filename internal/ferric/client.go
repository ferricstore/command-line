// Package ferric provides the CLI boundary around the FerricStore Go SDK.
package ferric

import ferricstore "github.com/ferricstore/ferricstore-go"

// DefaultURL is the local FerricStore endpoint used when no URL is configured.
const DefaultURL = "ferric://127.0.0.1:6388"

// NewClient constructs a FerricStore SDK client from a native or HTTP URL.
func NewClient(rawURL string) (*ferricstore.Client, error) {
	if rawURL == "" {
		rawURL = DefaultURL
	}
	return ferricstore.NewClientFromURL(rawURL)
}
