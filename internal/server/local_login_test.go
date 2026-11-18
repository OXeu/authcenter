package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
)

func localFlows(t *testing.T) (flow *flows, sentCode *string) {
	t.Helper()
	srv, _, _ := testServer(t, false)
	code := ""
	mail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-resend-key" || r.Header.Get("Idempotency-Key") == "" {
			t.Error("Resend headers missing")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		code = regexp.MustCompile(`\d{6}`).FindString(body["text"].(string))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"email-id"}`))
	}))
	t.Cleanup(mail.Close)
	srv.cfg.LocalLogin = true
	srv.cfg.EncryptionKey = []byte(strings.Repeat("k", 32))
	srv.cfg.ResendKey = "test-resend-key"
	srv.cfg.ResendFrom = "Auth <auth@example.com>"
	srv.mailer = credentials.Resend{APIKey: srv.cfg.ResendKey, From: srv.cfg.ResendFrom, Endpoint: mail.URL, Client: mail.Client()}
	return &flows{t: t, srv: srv, router: srv.router(), cookies: map[string]*http.Cookie{}}, &code
}

func registerLocal(t *testing.T, f *flows, code *string) {
	t.Helper()
	response := f.request("POST", "/auth/local/password", map[string]any{"email": "alice@example.com", "name": "Alice", "password": "correct horse battery", "register": true})
	if response.Code != 200 {
		t.Fatalf("register: %d %s", response.Code, response.Body.String())
	}
	if f.cookies["ac_session"] != nil || len(*code) != 6 {
		t.Fatal("session issued before email verification")
	}
	verified := f.request("POST", "/auth/local/verify", map[string]any{"method": "email", "code": *code})
	if verified.Code != 200 {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
	}
	me := f.request("GET", "/api/v1/me", nil)
	f.csrf = responseJSON(t, me)["csrf"].(string)
}

func TestPasswordEmailAndTOTPLogin(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	if f.request("POST", "/auth/local/verify", map[string]any{"method": "email", "code": *code}).Code != 401 {
		t.Fatal("email code replay accepted")
	}
	enrolled := f.request("POST", "/auth/local/totp", map[string]any{"password": "correct horse battery"})
	if enrolled.Code != 200 {
		t.Fatalf("enroll: %d %s", enrolled.Code, enrolled.Body.String())
	}
	setup := responseJSON(t, enrolled)
	secret := setup["secret"].(string)
	previous, err := credentials.TOTP(secret, time.Now().Unix()/30-1)
	if err != nil {
		t.Fatal(err)
	}
	verified := f.request("POST", "/auth/local/totp/verify", map[string]any{"request": setup["request"], "code": previous})
	if verified.Code != 204 {
		t.Fatalf("confirm enrollment: %d %s", verified.Code, verified.Body.String())
	}
	user, err := f.srv.store.UserByEmail(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(devices) != 1 {
		t.Fatal("missing encrypted device", err)
	}
	if devices[0].Secret == secret || user.PasswordHash == "correct horse battery" || !user.EmailVerified {
		t.Fatal("credential storage is not protected")
	}
	f.cookies = map[string]*http.Cookie{}
	f.csrf = ""
	password := map[string]any{"email": "alice@example.com", "password": "correct horse battery"}
	response := f.request("POST", "/auth/local/password", password)
	if response.Code != 200 || f.cookies["ac_session"] != nil {
		t.Fatal("password should only start a challenge")
	}
	current, err := credentials.TOTP(secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	response = f.request("POST", "/auth/local/verify", map[string]any{"method": "totp", "code": current})
	if response.Code != 200 {
		t.Fatalf("TOTP login: %d %s", response.Code, response.Body.String())
	}
	f.cookies = map[string]*http.Cookie{}
	f.csrf = ""
	f.request("POST", "/auth/local/password", password)
	if f.request("POST", "/auth/local/verify", map[string]any{"method": "totp", "code": current}).Code != 401 {
		t.Fatal("TOTP step replay accepted")
	}
}

func TestEmailChallengeAttemptsBindingAndRateLimit(t *testing.T) {
	f, code := localFlows(t)
	response := f.request("POST", "/auth/local/password", map[string]any{"email": "alice@example.com", "name": "Alice", "password": "correct horse battery", "register": true})
	if response.Code != 200 {
		t.Fatalf("register: %d", response.Code)
	}
	other := &flows{t: t, router: f.router, cookies: map[string]*http.Cookie{}}
	if other.request("POST", "/auth/local/verify", map[string]any{"method": "email", "code": *code}).Code != 401 {
		t.Fatal("challenge not bound to browser")
	}
	if f.request("POST", "/auth/local/email", nil).Code != 429 {
		t.Fatal("email resend not rate limited")
	}
	for range 5 {
		if f.request("POST", "/auth/local/verify", map[string]any{"method": "email", "code": "invalid"}).Code != 401 {
			t.Fatal("wrong code accepted")
		}
	}
	if f.request("POST", "/auth/local/verify", map[string]any{"method": "email", "code": *code}).Code != 401 {
		t.Fatal("exhausted challenge accepted")
	}
	if _, err := f.srv.store.UserByEmail(context.Background(), "alice@example.com"); err == nil {
		t.Fatal("unverified registration created an account")
	}
}

func TestLocalLoginCSRFAndExpiredChallenge(t *testing.T) {
	f, _ := localFlows(t)
	req := httptest.NewRequest("POST", "/auth/local/password", strings.NewReader(`{"email":"a@example.com","password":"correct horse battery"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("X-Authcenter-Request", "1")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	if response.Code != 403 {
		t.Fatal("login CSRF accepted")
	}
	challenge := store.LoginChallenge{ID: "expired", Purpose: "register", Email: "a@example.com", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := f.srv.store.CreateLoginChallenge(context.Background(), challenge); err != nil {
		t.Fatal(err)
	}
	f.cookies[loginChallengeCookie] = &http.Cookie{Name: loginChallengeCookie, Value: challenge.ID}
	if f.request("POST", "/auth/local/verify", map[string]any{"code": "123456", "method": "email"}).Code != 401 {
		t.Fatal("expired challenge accepted")
	}
}

func TestResendFailureCannotCreateSession(t *testing.T) {
	f, _ := localFlows(t)
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer unavailable.Close()
	f.srv.mailer.Endpoint = unavailable.URL
	response := f.request("POST", "/auth/local/password", map[string]any{"email": "alice@example.com", "name": "Alice", "password": "correct horse battery", "register": true})
	if response.Code != 502 || f.cookies["ac_session"] != nil {
		t.Fatal("delivery failure authenticated user")
	}
}
