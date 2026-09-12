// Package endpoint validates and safely renders connection endpoints.
package endpoint

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate rejects empty, invalid, or credential-bearing URLs and returns the
// trimmed value. Authentication material must travel through an explicit
// credential source rather than URL user information.
func Validate(raw, name string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s is invalid", name)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%s must not contain credentials", name)
	}
	return raw, nil
}

// Display returns an endpoint safe for terminal, JSON, and log output. It is
// deliberately defensive for legacy files and third-party providers: all URL
// user information and query values are removed even when validation should
// already have rejected them.
func Display(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[invalid endpoint]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}
