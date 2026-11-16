package store

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
)

func TestRegistrationPolicy(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, sqlite.Open(":memory:"), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := s.RegistrationAllowed(ctx)
	if err != nil || !allowed {
		t.Fatalf("default policy: %v, %v", allowed, err)
	}
	existing := Identity{Issuer: "https://example.com", Sub: "existing", Email: "existing@example.com"}
	if _, err := s.LinkOAuthUser(ctx, existing); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRegistrationAllowed(ctx, false, 1); err != nil {
		t.Fatal(err)
	}
	allowed, err = s.RegistrationAllowed(ctx)
	if err != nil || allowed {
		t.Fatalf("saved policy: %v, %v", allowed, err)
	}
	if _, err := s.LinkOAuthUser(ctx, existing); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkOAuthUser(ctx, Identity{Issuer: "https://example.com", Sub: "admin", Email: "admin@example.com"}); err != nil {
		t.Fatal(err)
	}
	newcomer := Identity{Issuer: "https://example.com", Sub: "new", Email: "new@example.com", Role: RoleAdmin}
	if _, err := s.LinkOAuthUser(ctx, newcomer); !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("registration error: %v", err)
	}
	if _, err := s.UserByEmail(ctx, newcomer.Email); !errors.Is(err, ErrNotFound) {
		t.Fatalf("blocked user exists: %v", err)
	}
	if err := s.SetRegistrationAllowed(ctx, true, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkOAuthUser(ctx, newcomer); err != nil {
		t.Fatal(err)
	}
}
