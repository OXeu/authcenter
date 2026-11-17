package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/config"
	"github.com/OXeu/center-common/oauth"
)

func TestMain(m *testing.M) {
	os.Exit(runWithScratch(m))
}

type flows struct {
	srv     *Server
	t       *testing.T
	router  http.Handler
	cookies map[string]*http.Cookie
	csrf    string
}

func (f *flows) do(method, path string, body any) (status int, decoded map[string]any) {
	f.t.Helper()
	var payload strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			f.t.Fatalf("encode body: %v", err)
		}
		payload = *strings.NewReader(string(encoded))
	}
	req := httptest.NewRequest(method, path, &payload)
	if len(f.cookies) > 0 {
		parts := make([]string, 0, len(f.cookies))
		for name, c := range f.cookies {
			parts = append(parts, url.QueryEscape(name)+"="+url.QueryEscape(c.Value))
		}
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
	if f.csrf != "" {
		req.Header.Set("X-CSRF-Token", f.csrf)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(f.cookies, c.Name)
			continue
		}
		f.cookies[c.Name] = c
	}
	_ = json.Unmarshal(w.Body.Bytes(), &decoded)
	return w.Code, decoded
}

func newFlows(t *testing.T) *flows {
	t.Helper()
	gin.SetMode(gin.TestMode)
	idp := newFakeIdP(t)
	cfg := config.Config{
		AllowRegistration: true,
		Addr:              "127.0.0.1:0",
		DatabaseDSN:       fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()),
		PublicURL:         "http://cc.test",
		SessionTTL:        time.Hour,
		Languages:         []string{"en", "zh-CN"},
		Bootstrap:         config.Bootstrap{AdminEmail: "admin@localhost", AdminName: "admin"},
		Groups:            auth.GroupPolicy{AdminGroups: []string{"authcenter-admin"}},
		OAuth: auth.Config{
			IssuerURL: idp.server.URL, ClientID: "client", ClientSecret: "secret",
			RedirectURL: "http://cc.test/oauth/callback", GroupsClaim: "groups",
		},
	}
	srv, err := newServer(context.Background(), cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	f := &flows{srv: srv, t: t, router: srv.router(), cookies: map[string]*http.Cookie{}}

	code, _ := f.do(http.MethodGet, "/auth/login", nil)
	if code != http.StatusFound {
		t.Fatalf("login status = %d", code)
	}
	state, nonce := startState(t, f)
	idp.lastNonce = nonce
	code, _ = f.do(http.MethodGet, "/oauth/callback?code=good&state="+url.QueryEscape(state), nil)
	if code != http.StatusFound {
		t.Fatalf("callback status = %d", code)
	}
	_, me := f.do(http.MethodGet, "/api/v1/me", nil)
	token, _ := me["csrf"].(string)
	f.csrf = token
	return f
}

func startState(t *testing.T, f *flows) (state, nonce string) {
	t.Helper()
	raw := f.cookies[oauthStateCookie]
	if raw == nil {
		t.Fatal("missing oauth state cookie")
	}
	sc, err := oauth.DecodeState(raw.Value)
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return sc.State, sc.Nonce
}
