package store

import (
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestMigrateAccountEmailsPreservesPrimaryAndVerification(t *testing.T) {
	dsn := "file:" + rand.Text() + "?mode=memory&cache=shared"
	s := registrationStore(t, dsn)
	legacy := User{Email: "legacy@example.com", EmailVerified: true, Name: "Legacy", Role: RoleUser}
	seedRegistrationRecords(t, s, &legacy)
	for range 2 {
		s = registrationStore(t, dsn)
		emails, err := s.AccountEmails(t.Context(), legacy.ID)
		if err != nil || len(emails) != 1 || !emails[0].Primary || !emails[0].Verified || emails[0].Email != legacy.Email {
			t.Fatal("email migration changed identity", emails, err)
		}
	}
	if err := s.createAccountEmail(t.Context(), legacy.ID, "secondary@example.com", true); err != nil {
		t.Fatal(err)
	}
	emails, err := s.AccountEmails(t.Context(), legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrimaryEmail(t.Context(), legacy, emails[1].ID, ""); err != nil {
		t.Fatal(err)
	}
	s = registrationStore(t, dsn)
	emails, err = s.AccountEmails(t.Context(), legacy.ID)
	if err != nil || len(emails) != 2 || emails[0].Primary || !emails[1].Primary {
		t.Fatal("restart reset selected primary", emails, err)
	}
}

func TestSecondaryEmailUniquenessAndOAuthIdentity(t *testing.T) {
	s := registrationStore(t, "file:"+rand.Text()+"?mode=memory&cache=shared")
	user, err := s.CreateUser(t.Context(), User{Email: "original@example.com", EmailVerified: true, Name: "User", Role: RoleUser, OAuthIssuer: "https://idp.example", OAuthSub: "subject"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.createAccountEmail(t.Context(), user.ID, "selected@example.com", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(t.Context(), User{Email: "selected@example.com", Name: "Duplicate", Role: RoleUser}); !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatal("secondary address reused by account creation", err)
	}
	if _, err := s.LinkOAuthUser(t.Context(), Identity{Email: "selected@example.com", EmailVerified: true, Issuer: "https://attacker.example", Sub: "other", Name: "Other", Now: time.Now()}); !errors.Is(err, ErrIdentityConflict) {
		t.Fatal("secondary address adopted by another identity", err)
	}
	emails, err := s.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrimaryEmail(t.Context(), user, emails[1].ID, ""); err != nil {
		t.Fatal(err)
	}
	updated, err := s.LinkOAuthUser(t.Context(), Identity{Email: user.Email, EmailVerified: false, Issuer: user.OAuthIssuer, Sub: user.OAuthSub, Name: user.Name, Now: time.Now()})
	if err != nil || updated.Email != "selected@example.com" || !updated.EmailVerified {
		t.Fatal("provider replaced selected primary", err)
	}
}

func TestPrimaryEmailCannotBeUnverifiedOrRemovedConcurrently(t *testing.T) {
	s := registrationStore(t, "file:"+rand.Text()+"?mode=memory&cache=shared")
	user, err := s.CreateUser(t.Context(), User{Email: "original@example.com", Name: "User", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	emails, err := s.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrimaryEmail(t.Context(), user, emails[0].ID, ""); !errors.Is(err, ErrPrimaryEmail) {
		t.Fatal("unverified primary selection accepted", err)
	}
	if err := s.createAccountEmail(t.Context(), user.ID, "verified@example.com", true); err != nil {
		t.Fatal(err)
	}
	emails, err = s.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Go(func() { results <- s.SetPrimaryEmail(t.Context(), user, emails[1].ID, "") })
	wait.Go(func() { results <- s.RemoveAccountEmail(t.Context(), user, emails[1].ID, "") })
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrPrimaryEmail) && !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("conflicting primary mutations succeeded", successes)
	}
	emails, err = s.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	primaries := 0
	for _, email := range emails {
		if email.Primary {
			primaries++
		}
	}
	if primaries != 1 {
		t.Fatal("primary email invariant lost")
	}
}

func TestDeleteUserReleasesEmailBindings(t *testing.T) {
	s := registrationStore(t, "file:"+rand.Text()+"?mode=memory&cache=shared")
	admin, err := s.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.CreateUser(t.Context(), User{Email: "delete@example.com", Name: "User", Role: RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.createAccountEmail(t.Context(), user.ID, "secondary@example.com", true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(t.Context(), admin.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(t.Context(), User{Email: "secondary@example.com", Name: "New", Role: RoleUser}); err != nil {
		t.Fatal("deleted account retained email", err)
	}
}

func TestRemovedEmailCannotCreateOrCompleteLoginChallenge(t *testing.T) {
	s := registrationStore(t, "file:"+rand.Text()+"?mode=memory&cache=shared")
	user, err := s.CreateUser(t.Context(), User{Email: "original@example.com", Name: "User", Role: RoleUser, PasswordHash: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.createAccountEmail(t.Context(), user.ID, "secondary@example.com", true); err != nil {
		t.Fatal(err)
	}
	emails, err := s.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	challenge := LoginChallenge{ID: "pending-email-login", UserID: user.ID, Email: emails[1].Email, Purpose: "login", PasswordHash: user.PasswordHash, EmailHash: "code", ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.CreateLoginChallenge(t.Context(), challenge); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAccountEmail(t.Context(), user, emails[1].ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifyLoginChallenge(t.Context(), challenge.ID, "code", 0, false, 0); err == nil {
		t.Fatal("removed email's login completed")
	}
	if err := s.CreateLoginChallenge(t.Context(), challenge); err == nil {
		t.Fatal("stale password request recreated a removed email challenge")
	}
}
