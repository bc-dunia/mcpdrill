package types

import "testing"

func TestTargetConfigGetHeadersWithBearerTokenRef(t *testing.T) {
	t.Setenv("MCPDRILL_TEST_AUTH_TOKEN", "secret-token")
	target := TargetConfig{
		Headers: map[string]string{"X-Existing": "kept"},
		Auth: &AuthConfig{
			Type:           "bearer_token",
			BearerTokenRef: "env://MCPDRILL_TEST_AUTH_TOKEN",
		},
	}

	headers := target.GetHeadersWithAuth()
	if headers["Authorization"] != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", headers["Authorization"])
	}
	if headers["X-Existing"] != "kept" {
		t.Fatalf("existing header was not preserved: %+v", headers)
	}
}

func TestTargetConfigGetHeadersWithAPIKeyHeaderRef(t *testing.T) {
	t.Setenv("MCPDRILL_TEST_API_KEY", "api-secret")
	target := TargetConfig{
		Auth: &AuthConfig{
			Type:             "api_key_header",
			APIKeyHeaderName: "X-Custom-Key",
			APIKeyRef:        "env://MCPDRILL_TEST_API_KEY",
		},
	}

	headers := target.GetHeadersWithAuth()
	if headers["X-Custom-Key"] != "api-secret" {
		t.Fatalf("X-Custom-Key = %q", headers["X-Custom-Key"])
	}
}
