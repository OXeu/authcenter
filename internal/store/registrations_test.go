package store

import (
	"crypto/rand"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
)

func registrationStore(t *testing.T, dsn string) *Store {
	t.Helper()
	s, err := Open(t.Context(), sqlite.Open(dsn), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := s.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func seedRegistrationRecords(t *testing.T, s *Store, records ...any) {
	t.Helper()
	for _, record := range records {
		if err := s.db.WithContext(t.Context()).Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func assertRegistrationCount(t *testing.T, s *Store, id string, want int64) {
	t.Helper()
	var count int64
	if err := s.db.WithContext(t.Context()).Model(&OAuthRegistration{}).Where("id = ?", id).Count(&count).Error; err != nil || count != want {
		t.Fatalf("registration %s count = %d, want %d: %v", id, count, want, err)
	}
}

func TestMigrateOAuthRegistrations(t *testing.T) {
	dsn := "file:" + rand.Text() + "?mode=memory&cache=shared"
	s := registrationStore(t, dsn)
	user, err := s.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	legacy := OAuthClient{ID: "legacy", Name: "Cursor", Public: true, Dynamic: true, RedirectURIs: []string{"cursor:/callback"}, Scopes: []string{"mcp", "offline_access"}, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	disabled := legacy
	disabled.ID, disabled.Disabled = "disabled", true
	manual := legacy
	manual.ID, manual.Dynamic, manual.OwnerID = "manual", false, user.ID
	grant := OAuthGrant{ID: "existing-grant", ClientID: legacy.ID, UserID: user.ID, Scope: "mcp offline_access", Resource: "https://auth.example/mcp"}
	refresh := "existing-refresh"
	token := OAuthToken{AccessHash: "existing-access", RefreshHash: &refresh, GrantID: grant.ID, ExpiresAt: time.Now().Add(time.Hour), RefreshExpiresAt: time.Now().Add(time.Hour)}
	request := AuthorizationRequest{ID: "pending", ClientID: legacy.ID, UserID: user.ID, CSRFHash: "csrf", RedirectURI: legacy.RedirectURIs[0], ExpiresAt: time.Now().Add(time.Hour)}
	code := AuthorizationCode{Digest: "existing-code", GrantID: grant.ID, ClientID: legacy.ID, RedirectURI: legacy.RedirectURIs[0], Challenge: "challenge", ExpiresAt: time.Now().Add(time.Hour)}
	seedRegistrationRecords(t, s, &legacy, &disabled, &manual, &grant, &token, &request, &code)
	s = registrationStore(t, dsn)
	assertRegistrationCount(t, s, legacy.ID, 1)
	s = registrationStore(t, dsn)
	var legacyCount int64
	if err := s.db.WithContext(t.Context()).Model(&OAuthClient{}).Where("dynamic = ?", true).Count(&legacyCount).Error; err != nil || legacyCount != 0 {
		t.Fatal("legacy applications remain after migration", legacyCount, err)
	}
	client, err := s.AuthorizationClientByID(t.Context(), legacy.ID)
	if err != nil || !reflect.DeepEqual(client, legacy) {
		t.Fatal("migration changed registration metadata", client, err)
	}
	if _, err := s.AuthorizationClientByID(t.Context(), disabled.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("migration restored a disabled registration", err)
	}
	clients, err := s.ListOAuthClients(t.Context())
	if err != nil || len(clients) != 1 || clients[0].ID != manual.ID {
		t.Fatal("migration changed application management", clients, err)
	}
	if _, err := s.AccessTokenGrant(t.Context(), token.AccessHash); err != nil {
		t.Fatal("migration invalidated an access token", err)
	}
	if _, err := s.RedeemAuthorizationCode(t.Context(), code.Digest, legacy.ID, code.RedirectURI, code.Challenge, grant.Resource, OAuthToken{AccessHash: "redeemed", ExpiresAt: token.ExpiresAt}); err != nil {
		t.Fatal("migration invalidated an authorization code", err)
	}
	if _, err := s.RotateOAuthToken(t.Context(), refresh, legacy.ID, grant.Resource, OAuthToken{AccessHash: "rotated", ExpiresAt: token.ExpiresAt}); err != nil {
		t.Fatal("migration invalidated a refresh token", err)
	}
	additional := grant
	additional.ID = "pending-grant"
	pendingCode := code
	pendingCode.Digest, pendingCode.GrantID = "pending-code", additional.ID
	if err := s.DecideAuthorization(t.Context(), request, additional, pendingCode, true); err != nil {
		t.Fatal("migration invalidated pending consent", err)
	}
	if err := s.RevokeOAuthGrant(t.Context(), user.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	assertRegistrationCount(t, s, legacy.ID, 1)
	if err := s.RevokeOAuthGrant(t.Context(), user.ID, additional.ID); err != nil {
		t.Fatal(err)
	}
	assertRegistrationCount(t, s, legacy.ID, 0)
}

func TestOAuthRegistrationRevocationLifecycle(t *testing.T) {
	for _, action := range []string{"grant", "access-token", "refresh-token", "code-replay", "refresh-replay", "password", "totp", "email", "ban", "delete-user"} {
		t.Run(action, func(t *testing.T) {
			s := registrationStore(t, "file:"+rand.Text()+"?mode=memory&cache=shared")
			admin, err := s.UserByEmail(t.Context(), "admin@example.com")
			if err != nil {
				t.Fatal(err)
			}
			user, err := s.CreateUser(t.Context(), User{Email: "user@example.com", Name: "User", Role: RoleUser, TOTPSecret: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			registration := OAuthRegistration{ID: "registered", Name: "Cursor", RedirectURIs: []string{"cursor:/callback"}, Scopes: []string{"mcp", "offline_access"}}
			manual := OAuthClient{ID: "manual", Name: "Manual", Public: true, OwnerID: admin.ID}
			grant := OAuthGrant{ID: "grant", ClientID: registration.ID, UserID: user.ID, Scope: "mcp offline_access", Resource: "https://auth.example/mcp"}
			refresh := "refresh"
			token := OAuthToken{AccessHash: "access", RefreshHash: &refresh, GrantID: grant.ID, ExpiresAt: time.Now().Add(time.Hour), RefreshExpiresAt: time.Now().Add(time.Hour), Used: true}
			code := AuthorizationCode{Digest: "code", ClientID: registration.ID, GrantID: grant.ID, RedirectURI: registration.RedirectURIs[0], Challenge: "challenge", Used: true, ExpiresAt: time.Now().Add(time.Hour)}
			seedRegistrationRecords(t, s, &registration, &manual, &grant, &token, &code)
			switch action {
			case "grant":
				err = s.RevokeOAuthGrant(t.Context(), user.ID, grant.ID)
			case "access-token":
				err = s.RevokeOAuthToken(t.Context(), token.AccessHash, registration.ID)
			case "refresh-token":
				err = s.RevokeOAuthToken(t.Context(), refresh, registration.ID)
			case "code-replay":
				_, err = s.RedeemAuthorizationCode(t.Context(), code.Digest, registration.ID, code.RedirectURI, code.Challenge, grant.Resource, OAuthToken{})
			case "refresh-replay":
				_, err = s.RotateOAuthToken(t.Context(), refresh, registration.ID, grant.Resource, OAuthToken{})
			case "password":
				err = s.SetPassword(t.Context(), user, "changed", "")
			case "totp":
				devices, loadErr := s.TOTPDevices(t.Context(), user.ID)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				err = s.DisableTOTP(t.Context(), user, devices[0].ID, 1, "", true)
			case "email":
				challenge := LoginChallenge{ID: "email-change", UserID: user.ID, SessionHash: "session", EmailHash: "code", Purpose: "email_change", PreviousEmail: user.Email, Email: "changed@example.com", ExpiresAt: time.Now().Add(time.Hour)}
				seedRegistrationRecords(t, s, &challenge)
				err = s.VerifyEmailChange(t.Context(), user.ID, challenge.ID, "session", "code", "", "email_change")
			case "ban":
				until := time.Now().Add(time.Hour)
				err = s.SetUserBan(t.Context(), admin.ID, user.ID, &until)
			case "delete-user":
				err = s.DeleteUser(t.Context(), admin.ID, user.ID)
			}
			if action == "code-replay" || action == "refresh-replay" {
				if !errors.Is(err, ErrInvalidGrant) {
					t.Fatal("replay was not rejected", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			assertRegistrationCount(t, s, registration.ID, 0)
			if _, err := s.AccessTokenGrant(t.Context(), token.AccessHash); err == nil {
				t.Fatal("revoked authorization remains usable")
			}
			if _, err := s.OAuthClientByID(t.Context(), manual.ID); err != nil {
				t.Fatal("revocation removed a manual application", err)
			}
		})
	}
}
