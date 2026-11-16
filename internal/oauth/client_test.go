package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFinishReportsTokenResponseErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "provider error with success status",
			status: http.StatusOK,
			body:   `{"error":"invalid_grant","error_description":"authorization code expired"}`,
			want:   "invalid oauth token: invalid_grant: authorization code expired",
		},
		{
			name:   "provider error with failure status",
			status: http.StatusBadRequest,
			body:   `{"error":"invalid_client","error_description":"client authentication failed"}`,
			want:   "invalid oauth token: invalid_client: client authentication failed",
		},
		{
			name:   "empty success response",
			status: http.StatusOK,
			body:   `{}`,
			want:   "response contains neither id_token nor access_token",
		},
		{
			name:   "non-json failure response",
			status: http.StatusBadGateway,
			body:   `upstream unavailable`,
			want:   "status 502",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			c := New(Config{}, srv.Client())
			c.remember(Discovery{TokenEndpoint: srv.URL})
			blob, err := json.Marshal(stateCookie{State: "state", Verifier: "verifier", Nonce: "nonce"})
			if err != nil {
				t.Fatalf("marshal state: %v", err)
			}
			stateBlob, err := encodeState(blob)
			if err != nil {
				t.Fatalf("encode state: %v", err)
			}

			_, err = c.Finish(context.Background(), "code", "state", stateBlob)
			if !errors.Is(err, ErrInvalidToken) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Finish() error = %v, want %q", err, tc.want)
			}
			if errors.Is(err, ErrNoIDToken) {
				t.Fatalf("Finish() error = %v, must not enter userinfo fallback", err)
			}
		})
	}
}

func TestUserinfoReportsMissingInputs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		meta        *Discovery
		accessToken string
		want        string
	}{
		{
			name:        "access token",
			meta:        &Discovery{UserinfoEndpoint: "https://idp.example.com/userinfo"},
			accessToken: "",
			want:        "missing access token",
		},
		{
			name:        "endpoint",
			meta:        &Discovery{},
			accessToken: "token",
			want:        "userinfo endpoint is not configured",
		},
		{
			name:        "metadata",
			meta:        nil,
			accessToken: "token",
			want:        "userinfo endpoint is not configured",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := New(Config{}, nil)
			if tc.meta != nil {
				c.remember(*tc.meta)
			}
			_, err := c.userinfo(context.Background(), tc.accessToken)
			if !errors.Is(err, ErrInvalidToken) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("userinfo() error = %v, want %q", err, tc.want)
			}
		})
	}
}
