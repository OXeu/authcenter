package configcenter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/OXeu/configcenter/sdk/configcenter"
)

// TestAgainstRunningServer exercises the SDK against a live server. Set
// CC_TEST_URL and CC_TEST_KEY (an editor key of an existing project):
//
//	CC_TEST_URL=http://127.0.0.1:8080 CC_TEST_KEY=cc_... go test ./... -run Live
func TestAgainstRunningServer(t *testing.T) {
	baseURL, key := os.Getenv("CC_TEST_URL"), os.Getenv("CC_TEST_KEY")
	if baseURL == "" || key == "" {
		t.Skip("CC_TEST_URL and CC_TEST_KEY are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := configcenter.New(configcenter.Config{DotenvFiles: []string{}, BaseURL: baseURL, APIKey: key, Environment: "production"})
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
