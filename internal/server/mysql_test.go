//go:build mysql

package server

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/config"
	"github.com/OXeu/authcenter/internal/store"
)

func TestMySQLStore(t *testing.T) {
	ctx := context.Background()
	dialect, err := databaseDialector(config.Config{DatabaseDriver: "mysql", DatabaseDSN: "root@tcp(127.0.0.1:23306)/authcenter?parseTime=true"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dialect, "admin@example.com", "admin")
	if err != nil {
		t.Fatal(err)
	}
	client := store.OAuthClient{ID: rand.Text(), Name: "MySQL", Homepage: "https://app.example", RedirectURIs: []string{"https://app.example/cb"}, Scopes: []string{"profile", "offline_access"}}
	if err := st.CreateOAuthClient(ctx, client); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.DisableOAuthClient(ctx, client.ID); err != nil {
			t.Error(err)
		}
	})
	client.RedirectURIs = append(client.RedirectURIs, "https://app.example/second")
	if err := st.UpdateOAuthClient(ctx, client); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.OAuthClientByID(ctx, client.ID)
	if err != nil || len(loaded.RedirectURIs) != 2 {
		t.Fatalf("callbacks: %v %v", loaded.RedirectURIs, err)
	}
	request := store.AuthorizationRequest{ID: rand.Text(), ClientID: client.ID, RedirectURI: client.RedirectURIs[0], ExpiresAt: time.Now().Add(time.Minute)}
	if err := st.CreateAuthorizationRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	grant := store.OAuthGrant{ID: rand.Text(), ClientID: client.ID, Scope: "profile offline_access"}
	code := store.AuthorizationCode{Digest: digest(rand.Text()), GrantID: grant.ID, ClientID: client.ID, RedirectURI: request.RedirectURI, ExpiresAt: request.ExpiresAt}
	if err := st.DecideAuthorization(ctx, request, grant, code, true); err != nil {
		t.Fatal(err)
	}
	refreshHash := digest(rand.Text())
	token := store.OAuthToken{AccessHash: digest(rand.Text()), RefreshHash: &refreshHash, ExpiresAt: time.Now().Add(time.Hour), RefreshExpiresAt: time.Now().Add(24 * time.Hour)}
	if _, err := st.RedeemAuthorizationCode(ctx, code.Digest, client.ID, request.RedirectURI, "", "", token); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AccessTokenGrant(ctx, token.AccessHash); err != nil {
		t.Fatal(err)
	}
}
