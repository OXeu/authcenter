package configcenter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func dotenvTestFile(t *testing.T, contents string) string {
	t.Helper()
	path := localTestPath(t) + ".env"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func snapshotValues(items []Item) map[string]string {
	values := make(map[string]string, len(items))
	for _, item := range items {
		values[item.Key] = item.Value
	}
	return values
}

func TestFallbackPriorityAndEmptyValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"items": []Item{
			{Key: "remote", Value: "remote-value", Description: "remote metadata"},
			{Key: "remote_empty", Value: ""},
			{Key: "invalid_int", Value: "invalid"},
		}})
	}))
	defer server.Close()
	path := localTestPath(t)
	file := dotenvTestFile(t, "remote=dotenv\nremote_empty=dotenv\nlocal=dotenv\nlocal_empty=dotenv\ndotenv='dotenv value'\ndotenv_empty=\ninvalid_int=10\n")
	defaults := map[string]string{"remote": "default", "local": "default", "dotenv": "default", "dotenv_empty": "default", "default": "default-value", "invalid_int": "20"}
	cfg := Config{
		BaseURL: server.URL, APIKey: "cc_test", LocalPath: path,
		LocalDefaults: map[string]string{"remote": "local", "remote_empty": "local", "local": "local-value", "local_empty": ""},
		DotenvFiles:   []string{file}, Defaults: defaults,
	}
	client := localClient(t, cfg)
	defaults["default"] = "mutated"
	if err := client.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"remote": "remote-value", "remote_empty": "", "local": "local-value", "local_empty": "", "dotenv": "dotenv value", "dotenv_empty": "", "default": "default-value", "invalid_int": "invalid"}
	if got := snapshotValues(client.All()); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %#v, want %#v", got, want)
	}
	if client.GetDefault("missing", "caller") != "caller" || client.Int("invalid_int", 7) != 7 {
		t.Fatal("caller defaults not respected")
	}
	for _, item := range client.All() {
		if item.Key == "remote" && item.Description != "remote metadata" {
			t.Fatal("remote metadata lost")
		}
	}
	local := localClient(t, Config{LocalPath: path, DotenvFiles: []string{}})
	if err := local.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := snapshotValues(local.All()); !reflect.DeepEqual(got, cfg.LocalDefaults) {
		t.Fatalf("dotenv, defaults or project cache leaked into local fallback: %#v", got)
	}
}

func TestRemoteCacheSurvivesMissingKeysFailuresAndRestarts(t *testing.T) {
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case 1:
			json.NewEncoder(w).Encode(map[string]any{"items": []Item{}})
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			items := []Item{{Key: "db.pool", Value: "20"}, {Key: "feature.enabled", Value: "true"}}
			if r.URL.Query().Get("prefix") != "" {
				items = []Item{{Key: "db.pool", Value: "30"}}
			}
			json.NewEncoder(w).Encode(map[string]any{"items": items})
		}
	}))
	defer server.Close()
	cfg := Config{BaseURL: server.URL, APIKey: "cc_test", LocalPath: localTestPath(t), DotenvFiles: []string{}}
	client := localClient(t, cfg)
	if err := client.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := client.LoadPrefix(t.Context(), "db."); err != nil {
		t.Fatal(err)
	}
	if len(client.All()) != 1 || client.Int("db.pool", 0) != 30 {
		t.Fatal("prefix snapshot not replaced")
	}
	for _, state := range []int32{1, 2} {
		mode.Store(state)
		reopened := localClient(t, cfg)
		err := reopened.Load(t.Context())
		if (err != nil) != (state == 2) {
			t.Fatalf("state %d: error = %v", state, err)
		}
		if reopened.Int("db.pool", 0) != 30 || !reopened.Bool("feature.enabled", false) {
			t.Fatal("cache not persisted or prefix refresh erased unrelated keys")
		}
		if err := reopened.LoadPrefix(t.Context(), "feature."); (err != nil) != (state == 2) {
			t.Fatal(err)
		}
		if len(reopened.All()) != 1 {
			t.Fatal("fallback ignored prefix")
		}
	}
	server.Close()
	if err := client.Load(t.Context()); err == nil || !client.Bool("feature.enabled", false) {
		t.Fatal("transport failure did not expose cached values", err)
	}
}

func TestRemoteCacheScopeIsolation(t *testing.T) {
	var offline atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if offline.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []Item{{Key: "secret", Value: "private"}}})
	}))
	defer server.Close()
	cfg := Config{BaseURL: server.URL, APIKey: "cc_one", LocalPath: localTestPath(t), DotenvFiles: []string{}}
	if err := localClient(t, cfg).Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	offline.Store(true)
	for _, change := range []func(*Config){
		func(c *Config) { c.APIKey = "cc_two" },
		func(c *Config) { c.Environment = "staging" },
		func(c *Config) { c.BaseURL += "/other" },
		func(c *Config) { c.BaseURL, c.APIKey = "", "" },
	} {
		other := cfg
		change(&other)
		client := localClient(t, other)
		err := client.Load(t.Context())
		if !client.IsLocal() && err == nil {
			t.Fatal("remote failure hidden")
		}
		if _, ok := client.Get("secret"); ok {
			t.Fatal("cached remote value crossed a scope boundary")
		}
	}
}

func TestDotenvPriorityReloadAndErrors(t *testing.T) {
	t.Setenv("SDK_ENV_UNCHANGED", "process")
	first := dotenvTestFile(t, "export SDK_ENV_UNCHANGED=first\nBASE=hello\nexpanded=\"${BASE} world\"\nempty=\n")
	second := dotenvTestFile(t, "SDK_ENV_UNCHANGED=second\nempty=second\nother=value\n")
	client := localClient(t, Config{LocalPath: localTestPath(t), DotenvFiles: []string{first, second}, Defaults: map[string]string{"default": "value"}})
	if err := client.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	if client.GetDefault("SDK_ENV_UNCHANGED", "") != "first" || client.GetDefault("expanded", "") != "hello world" || client.GetDefault("empty", "fallback") != "" {
		t.Fatal("dotenv parsing or file priority incorrect")
	}
	if os.Getenv("SDK_ENV_UNCHANGED") != "process" {
		t.Fatal("SDK mutated process environment")
	}
	if err := os.WriteFile(first, []byte("BASE=changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	if client.GetDefault("BASE", "") != "changed" || client.GetDefault("SDK_ENV_UNCHANGED", "") != "second" {
		t.Fatal("dotenv changes not reloaded")
	}
	if _, ok := client.Get("expanded"); ok {
		t.Fatal("deleted dotenv value persisted in KV")
	}
	secret := "sensitive-content"
	if err := os.WriteFile(first, []byte("!invalid="+secret), 0o600); err != nil {
		t.Fatal(err)
	}
	err := client.Load(t.Context())
	if !errors.Is(err, ErrInvalidDotenv) || strings.Contains(err.Error(), secret) {
		t.Fatal("dotenv error missing or exposed contents", err)
	}
	if client.GetDefault("other", "") != "value" || client.GetDefault("default", "") != "value" {
		t.Fatal("dotenv failure blocked lower sources")
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if err := client.Load(t.Context()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("explicit missing dotenv file was ignored", err)
	}
}

func TestOptionalAndDisabledDotenv(t *testing.T) {
	dir := filepath.Dir(localTestPath(t))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	client := localClient(t, Config{LocalPath: filepath.Join(dir, "cache.kv")})
	if err := client.Load(t.Context()); err != nil {
		t.Fatal("missing optional .env", err)
	}
	if err := os.WriteFile(".env", []byte("key=dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.Load(t.Context()); err != nil || client.GetDefault("key", "") != "dotenv" {
		t.Fatal("default .env not read", err)
	}
	disabled := localClient(t, Config{LocalPath: filepath.Join(dir, "cache.kv"), DotenvFiles: []string{}})
	if err := disabled.Load(t.Context()); err != nil || len(disabled.All()) != 0 {
		t.Fatal("empty file list did not disable dotenv", err)
	}
}

func TestCacheFailureStillExposesRemoteAndCancellationPreservesSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"items": []Item{{Key: "key", Value: "remote"}}})
	}))
	defer server.Close()
	path := dotenvTestFile(t, "not a database")
	client := localClient(t, Config{BaseURL: server.URL, APIKey: "cc_test", LocalPath: path, DotenvFiles: []string{}})
	if err := client.Load(t.Context()); err == nil || client.GetDefault("key", "") != "remote" {
		t.Fatal("cache failure discarded remote data or error", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.LoadPrefix(ctx, "other."); !errors.Is(err, context.Canceled) || client.GetDefault("key", "") != "remote" {
		t.Fatal("cancelled load replaced snapshot", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "not a database" {
		t.Fatal("corrupt KV was replaced", err)
	}
}

func TestRemotePutCachesOnlySuccessfulWrites(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/project"):
			json.NewEncoder(w).Encode(map[string]any{"project": map[string]string{"slug": "project"}})
		case r.Method == http.MethodPut:
			if fail.Load() {
				w.WriteHeader(http.StatusForbidden)
			}
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	cfg := Config{BaseURL: server.URL, APIKey: "cc_test", LocalPath: localTestPath(t), DotenvFiles: []string{}}
	client := localClient(t, cfg)
	if err := client.Put(t.Context(), "key", "written"); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := client.Put(t.Context(), "key", "rejected"); err == nil {
		t.Fatal("failed write accepted")
	}
	reopened := localClient(t, cfg)
	if err := reopened.Load(t.Context()); err == nil || reopened.GetDefault("key", "") != "written" {
		t.Fatal("cache did not preserve successful write", err)
	}
}

type failingSource struct {
	err error
}

func (s failingSource) Read(context.Context, string) ([]Item, error) {
	return nil, s.err
}

func TestDataSourceComposition(t *testing.T) {
	failure := errors.New("source unavailable")
	var source DataSource = Sources{
		MapSource{"db.empty": "", "other": "ignored"},
		failingSource{err: failure},
		MapSource{"db.empty": "default", "db.pool": "10"},
	}
	items, err := source.Read(t.Context(), "db.")
	if !errors.Is(err, failure) || !reflect.DeepEqual(snapshotValues(items), map[string]string{"db.empty": "", "db.pool": "10"}) {
		t.Fatal("composed source lost fallback or failure", items, err)
	}
}

func TestConcurrentRemoteLoadsAndWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/project"):
			json.NewEncoder(w).Encode(map[string]any{"project": map[string]string{"slug": "project"}})
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"items": []Item{{Key: "remote", Value: "value"}}})
		}
	}))
	defer server.Close()
	client := localClient(t, Config{BaseURL: server.URL, APIKey: "cc_test", LocalPath: localTestPath(t), DotenvFiles: []string{}})
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if err := client.Load(t.Context()); err != nil {
				t.Error(err)
			}
			if err := client.Put(t.Context(), "written", "value"); err != nil {
				t.Error(err)
			}
			client.All()
			client.Get("written")
		})
	}
	wg.Wait()
	if err := client.Load(t.Context()); err != nil || len(client.All()) != 2 {
		t.Fatal("concurrent operations lost cached writes", err)
	}
}

func TestFallbackCorruptRecordDoesNotPublishPartialData(t *testing.T) {
	client := localClient(t, Config{LocalPath: localTestPath(t), DotenvFiles: []string{}, Defaults: map[string]string{"default": "value"}})
	if err := client.local.transaction(t.Context(), func(bucket *bolt.Bucket) error {
		if err := writeLocalItem(bucket, Item{Key: "a", Value: "partial"}); err != nil {
			return err
		}
		return bucket.Put([]byte("z"), []byte("invalid"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.Load(t.Context()); err == nil {
		t.Fatal("corrupt record accepted")
	}
	if _, ok := client.Get("a"); ok || client.GetDefault("default", "") != "value" {
		t.Fatal("partial local data leaked or defaults blocked")
	}
}
