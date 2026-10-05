package provider

import (
	"os"
	"testing"
)

func requireLiveCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run live acceptance tests against a real App Store Connect team")
	}
	for _, name := range []string{"APP_STORE_CONNECT_ISSUER_ID", "APP_STORE_CONNECT_KEY_ID", "APP_STORE_CONNECT_PRIVATE_KEY"} {
		if os.Getenv(name) == "" {
			t.Skipf("set %s to run live acceptance tests against a real App Store Connect team", name)
		}
	}
}

func TestLive_Authenticates(t *testing.T) {
	requireLiveCredentials(t)

	credentials, err := newAPICredentials(
		os.Getenv("APP_STORE_CONNECT_ISSUER_ID"),
		os.Getenv("APP_STORE_CONNECT_KEY_ID"),
		os.Getenv("APP_STORE_CONNECT_PRIVATE_KEY"),
	)
	if err != nil {
		t.Fatalf("failed to load live credentials: %v", err)
	}
	tokens := newTokenSource(credentials)
	client := newAPIClient(defaultBaseURL, newRetryableClient(tokens), tokens)

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := client.get(t.Context(), "/v1/apps", map[string][]string{"limit": {"1"}}, &out); err != nil {
		t.Fatalf("expected an authenticated request to succeed, got: %v", err)
	}
}
