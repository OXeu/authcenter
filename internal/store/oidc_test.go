package store

import (
	"crypto/rand"
	"slices"
	"testing"
)

func TestOpenIDMigrationPreservesExistingClientsAndGrants(t *testing.T) {
	dsn := "file:" + rand.Text() + "?mode=memory&cache=shared"
	s := registrationStore(t, dsn)
	client := OAuthClient{ID: "existing", Name: "Existing", SecretHash: "unchanged", RedirectURIs: []string{"https://app.example/callback"}, Scopes: []string{"profile", "email"}}
	disabled := client
	disabled.ID, disabled.Disabled = "disabled", true
	registration := OAuthRegistration{ID: "dynamic", RedirectURIs: []string{"cursor:/callback"}, Scopes: []string{"mcp"}}
	grant := OAuthGrant{ID: "grant", ClientID: client.ID, Scope: "profile email"}
	seedRegistrationRecords(t, s, &client, &disabled, &registration, &grant)
	for range 2 {
		s = registrationStore(t, dsn)
		var clients []OAuthClient
		if err := s.db.WithContext(t.Context()).Find(&clients).Error; err != nil {
			t.Fatal(err)
		}
		for _, got := range clients {
			if !slices.Equal(got.Scopes, []string{"profile", "email", "openid"}) || got.SecretHash != "unchanged" ||
				!slices.Equal(got.RedirectURIs, client.RedirectURIs) || got.Disabled != (got.ID == "disabled") {
				t.Fatal("migration changed client metadata or repeated scope", got.ID)
			}
		}
	}
	if err := s.db.WithContext(t.Context()).First(&registration, "id = ?", registration.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(registration.Scopes, []string{"mcp"}) {
		t.Fatal("migration extended dynamic client access")
	}
	if err := s.db.WithContext(t.Context()).First(&grant, "id = ?", grant.ID).Error; err != nil || grant.Scope != "profile email" {
		t.Fatal("migration changed previously consented scopes", err)
	}
}
