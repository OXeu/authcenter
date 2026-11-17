package store

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
)

func TestLegacyKeysMigrateWithoutReplacingCredentials(t *testing.T) {
	ctx := t.Context()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	s, err := Open(ctx, sqlite.Open(dsn), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, Project{Slug: "legacy", Name: "Legacy", OwnerID: 1}, Environment{Slug: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Migrator().DropColumn(&APIKey{}, "Scopes"); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	for index, role := range []Role{RoleViewer, RoleEditor, RoleAdmin} {
		projectID := project.ID
		if role == RoleAdmin {
			projectID = 0
		}
		digest := bytes.Repeat([]byte{byte(index + 1)}, 32)
		if err := s.db.Table("api_keys").Create(map[string]any{"name": string(role), "role": role, "project_id": projectID, "digest": digest, "prefix": "cc_test", "created_by": 1, "created_at": time.Now().UTC(), "expires_at": expires}).Error; err != nil {
			t.Fatal(err)
		}
	}
	migrated, err := Open(ctx, sqlite.Open(dsn), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	for index, role := range []Role{RoleViewer, RoleEditor, RoleAdmin} {
		digest := [32]byte{}
		copy(digest[:], bytes.Repeat([]byte{byte(index + 1)}, 32))
		key, err := migrated.APIKeyByDigest(ctx, digest)
		if err != nil || key.CreatedBy != 1 || key.ExpiresAt == nil || !key.ExpiresAt.Equal(expires) {
			t.Fatal("migration altered key identity or expiration")
		}
		if !key.Allows(project.ID, ConfigRead) {
			t.Fatal("migration lost read access")
		}
		if key.Allows(project.ID, ConfigWrite) != (role != RoleViewer) {
			t.Fatal("migration changed write access")
		}
		if key.Allows(project.ID+100, ConfigRead) != (role == RoleAdmin) {
			t.Fatal("migration widened project access")
		}
		if key.LegacyProjectID != 0 || key.LegacyRole != RoleViewer {
			t.Fatal("legacy authorization fields were not retired")
		}
		if err := migrated.UpdateAPIKey(ctx, key.ID, key.Revision, key.Name, []KeyScope{}); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(ctx, sqlite.Open(dsn), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := reopened.ListAPIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if key.Scopes == nil || len(key.Scopes) != 0 || key.Allows(project.ID, ConfigRead) {
			t.Fatal("restart restored revoked scopes")
		}
	}
}

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
	if err := s.UpdateAPIKey(t.Context(), key.ID, key.Revision, "expanded", LegacyKeyScopes(0, RoleAdmin)); err != nil {
		t.Fatal(err)
	}
	if err := s.RotateAPIKey(t.Context(), key.ID, key.Revision, bytes.Repeat([]byte{2}, 32), "cc_new"); !errors.Is(err, ErrKeyChanged) {
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
