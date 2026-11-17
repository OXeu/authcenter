package authcenter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/OXeu/authcenter/sdk/authcenter"
)

// TestAgainstRunningServer exercises the SDK against a live server. Set
// AC_TEST_URL and AC_TEST_KEY (an editor key of an existing project):
//
//	AC_TEST_URL=http://127.0.0.1:8080 AC_TEST_KEY=ac_... go test ./... -run Live
func TestAgainstRunningServer(t *testing.T) {
	baseURL, key := os.Getenv("AC_TEST_URL"), os.Getenv("AC_TEST_KEY")
	if baseURL == "" || key == "" {
		t.Skip("AC_TEST_URL and AC_TEST_KEY are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := authcenter.New(authcenter.Config{DotenvFiles: []string{}, BaseURL: baseURL, APIKey: key, Environment: "production"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(client.All()) == 0 {
		t.Fatal("expected at least one item in production")
	}
	t.Logf("loaded %d items", len(client.All()))
}
