package store

import (
	"bytes"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
)

func TestProjectDeletionPreservesOtherKeyScopes(t *testing.T) {
	s, err := Open(t.Context(), sqlite.Open(":memory:"), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateProject(t.Context(), Project{Slug: "first", Name: "First", OwnerID: 1}, Environment{Slug: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateProject(t.Context(), Project{Slug: "second", Name: "Second", OwnerID: 1}, Environment{Slug: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(t.Context(), APIKey{Name: "multiple", Digest: bytes.Repeat([]byte{1}, 32), Scopes: []KeyScope{{ProjectID: a.ID, Permissions: []Permission{ConfigRead}}, {ProjectID: b.ID, Permissions: []Permission{ConfigWrite}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
	key, err = s.APIKeyByID(t.Context(), key.ID)
	if err != nil || len(key.Scopes) != 1 || !key.Allows(b.ID, ConfigWrite) || key.HasProject(a.ID) {
		t.Fatal("deletion did not isolate remaining scopes")
	}
	replacement, err := s.CreateProject(t.Context(), Project{Slug: "first", Name: "Reused slug", OwnerID: 1}, Environment{Slug: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	if key.HasProject(replacement.ID) {
		t.Fatal("recreating a slug restored old key access")
	}
	if err := s.DeleteProject(t.Context(), b.ID); err != nil {
		t.Fatal(err)
	}
	key, err = s.APIKeyByID(t.Context(), key.ID)
	if err != nil || key.Scopes == nil || len(key.Scopes) != 0 {
		t.Fatal("last removed project must leave an explicitly disabled key")
	}
}

func TestKeyMutationsRejectStaleAuthorization(t *testing.T) {
	s, err := Open(t.Context(), sqlite.Open(":memory:"), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.CreateAPIKey(t.Context(), APIKey{Name: "restricted", Digest: bytes.Repeat([]byte{1}, 32), Scopes: []KeyScope{{ProjectID: 1, Permissions: []Permission{ConfigRead}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAPIKey(t.Context(), key.ID, key.Revision, "expanded", []KeyScope{{Permissions: KeyPermissions()}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RotateAPIKey(t.Context(), key.ID, key.Revision, bytes.Repeat([]byte{2}, 32), "ac_new"); !errors.Is(err, ErrKeyChanged) {
		t.Fatal("stale authorization rotated a broader credential")
	}
	if err := s.UpdateAPIKey(t.Context(), key.ID, key.Revision, "overwrite", []KeyScope{}); !errors.Is(err, ErrKeyChanged) {
		t.Fatal("stale authorization overwrote new scopes")
	}
	if err := s.DeleteAPIKey(t.Context(), key.ID, key.Revision); !errors.Is(err, ErrKeyChanged) {
		t.Fatal("stale authorization deleted a broader key")
	}
	fresh, err := s.APIKeyByID(t.Context(), key.ID)
	if err != nil || !fresh.Allows(0, ProjectsWrite) || !bytes.Equal(fresh.Digest, key.Digest) {
		t.Fatal("rejected operations changed the key")
	}
}
