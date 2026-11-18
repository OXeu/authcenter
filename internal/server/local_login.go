package server

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
)

const loginChallengeCookie = "ac_login_challenge"

func (s *Server) mountLocalLogin(r *gin.Engine) {
	group := r.Group("/auth/local", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !s.cfg.LocalLogin {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if c.Request.Method != http.MethodGet && (c.GetHeader("X-Authcenter-Request") != "1" || c.GetHeader("Origin") != "" && c.GetHeader("Origin") != s.cfg.PublicURL) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	})
	group.POST("/password", s.passwordLogin)
	group.POST("/verify", s.verifyLogin)
	group.POST("/email", s.sendLoginEmail)
	group.GET("/security", auth.RequireUser(), s.securitySettings)
	group.POST("/totp", auth.RequireUser(), s.enrollTOTP)
	group.POST("/totp/verify", auth.RequireUser(), s.verifyTOTPEnrollment)
}

func (s *Server) rateLimit(c *gin.Context, category, key string, limit int, window int64) bool {
	bucket := strconv.FormatInt(time.Now().Unix()/window, 10)
	allowed, err := s.store.AllowAttempt(c.Request.Context(), digest(category+":"+key+":"+bucket), limit, time.Duration(window)*time.Second)
	if err != nil {
		s.fail(c, err)
		return false
	}
	if !allowed {
		c.Header("Retry-After", strconv.FormatInt(window, 10))
		oauthError(c, 429, "rate_limited")
		return false
	}
	return true
}

func (s *Server) passwordLogin(c *gin.Context) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Register bool   `json:"register"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 || len(in.Password) > 72 {
		oauthError(c, 400, "invalid_credentials")
		return
	}
	if !s.rateLimit(c, "password-ip", c.ClientIP(), 30, 900) || !s.rateLimit(c, "password-email", in.Email, 10, 900) {
		return
	}
	user, userErr := s.store.UserByEmail(c.Request.Context(), in.Email)
	if userErr != nil && !errors.Is(userErr, store.ErrNotFound) {
		s.fail(c, userErr)
		return
	}
	challenge := store.LoginChallenge{ID: rand.Text() + rand.Text(), Email: in.Email, UserID: user.ID, PasswordHash: user.PasswordHash, Purpose: "login", ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if userErr == nil && !user.Active() {
		oauthError(c, 401, "invalid_credentials")
		return
	}
	if in.Register {
		if !s.cfg.AllowRegistration || s.cfg.ResendKey == "" || s.cfg.ResendFrom == "" {
			oauthError(c, 403, "registration_unavailable")
			return
		}
		if userErr == nil && (user.PasswordHash != "" || user.OAuthSub != "") {
			oauthError(c, 400, "registration_unavailable")
			return
		}
		if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 {
			oauthError(c, 400, "invalid_name")
			return
		}
		hash, err := credentials.HashPassword(in.Password)
		if err != nil {
			oauthError(c, 400, "password_length")
			return
		}
		challenge.PasswordHash, challenge.Name, challenge.Purpose = hash, strings.TrimSpace(in.Name), "register"
	} else {
		hash := user.PasswordHash
		if hash == "" {
			hash = "$2a$12$Jq8nA63VETFb46nrGhrmXuHcLhyoZnxlXoT62sQs8cboRwgQrqGYe"
		}
		if !credentials.CheckPassword(hash, in.Password) || userErr != nil || user.PasswordHash == "" {
			oauthError(c, 401, "invalid_credentials")
			return
		}
	}
	devices, err := s.store.TOTPDevices(c.Request.Context(), user.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	if len(devices) == 0 && (s.cfg.ResendKey == "" || s.cfg.ResendFrom == "") {
		oauthError(c, 503, "second_factor_unavailable")
		return
	}
	if err := s.store.CreateLoginChallenge(c.Request.Context(), challenge); err != nil {
		s.fail(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: loginChallengeCookie, Value: challenge.ID, Path: "/auth/local", HttpOnly: true, Secure: c.GetBool("secureCookies"), SameSite: http.SameSiteStrictMode, MaxAge: 300})
	methods := []string{}
	if len(devices) > 0 {
		methods = append(methods, "totp")
	}
	if s.cfg.ResendKey != "" && s.cfg.ResendFrom != "" {
		methods = append(methods, "email")
	}
	if len(devices) == 0 {
		if !s.deliverChallenge(c, challenge) {
			return
		}
	}
	c.JSON(200, gin.H{"methods": methods})
}

func (s *Server) currentChallenge(c *gin.Context) (store.LoginChallenge, bool) {
	raw, err := c.Cookie(loginChallengeCookie)
	if err != nil {
		oauthError(c, 401, "invalid_challenge")
		return store.LoginChallenge{}, false
	}
	challenge, err := s.store.LoginChallengeByID(c.Request.Context(), raw)
	if err != nil || (challenge.Purpose != "login" && challenge.Purpose != "register") {
		oauthError(c, 401, "invalid_challenge")
		return store.LoginChallenge{}, false
	}
	return challenge, true
}

func (s *Server) deliverChallenge(c *gin.Context, challenge store.LoginChallenge) bool {
	if s.cfg.ResendKey == "" || s.cfg.ResendFrom == "" {
		oauthError(c, 503, "email_unavailable")
		return false
	}
	if !s.rateLimit(c, "email", challenge.Email, 1, 60) || !s.rateLimit(c, "email-daily", challenge.Email, 30, 86400) {
		return false
	}
	code, err := credentials.Code()
	if err != nil {
		s.fail(c, err)
		return false
	}
	if err := s.store.SetChallengeEmail(c.Request.Context(), challenge.ID, credentials.CodeHash(s.cfg.EncryptionKey, challenge.ID, code)); err != nil {
		s.fail(c, err)
		return false
	}
	if err := s.mailer.Send(c.Request.Context(), challenge.Email, code, rand.Text()); err != nil {
		s.log.Error("email delivery failed", "error", err)
		oauthError(c, 502, "email_delivery_failed")
		return false
	}
	return true
}

func (s *Server) sendLoginEmail(c *gin.Context) {
	challenge, ok := s.currentChallenge(c)
	if !ok {
		return
	}
	if s.deliverChallenge(c, challenge) {
		c.Status(http.StatusNoContent)
	}
}

func (s *Server) verifyLogin(c *gin.Context) {
	challenge, ok := s.currentChallenge(c)
	if !ok {
		return
	}
	var in struct {
		Code   string `json:"code"`
		Method string `json:"method"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	emailHash := ""
	step := int64(0)
	valid := false
	deviceID := uint(0)
	switch in.Method {
	case "email":
		emailHash = credentials.CodeHash(s.cfg.EncryptionKey, challenge.ID, in.Code)
	case "totp":
		devices, err := s.store.TOTPDevices(c.Request.Context(), challenge.UserID)
		if err != nil {
			s.fail(c, err)
			return
		}
		deviceID, step, valid = s.matchTOTP(devices, in.Code)
	default:
		oauthError(c, 400, "invalid_request")
		return
	}
	user, err := s.store.VerifyLoginChallenge(c.Request.Context(), challenge.ID, emailHash, step, valid, deviceID)
	if err != nil {
		oauthError(c, 401, "invalid_code")
		return
	}
	if err := s.authenticator.Login(c, user.ID); err != nil {
		s.fail(c, err)
		return
	}
	clearCookie(c, loginChallengeCookie, "/auth/local")
	c.JSON(200, gin.H{"redirect": s.loginDestination(c)})
}

func (s *Server) securitySettings(c *gin.Context) {
	user, _ := s.me(c)
	devices, err := s.store.TOTPDevices(c.Request.Context(), user.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	emails, err := s.store.AccountEmails(c.Request.Context(), user.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"email": user.Email, "emails": emails, "totp_devices": devices, "totp_enabled": len(devices) > 0, "password_enabled": user.PasswordHash != "", "local_login": s.cfg.LocalLogin, "email_available": s.cfg.ResendKey != "" && s.cfg.ResendFrom != "" && len(s.cfg.EncryptionKey) == 32})
}

func (s *Server) enrollTOTP(c *gin.Context) {
	user, _ := s.me(c)
	if !s.rateLimit(c, "enroll", strconv.FormatUint(uint64(user.ID), 10), 5, 900) {
		return
	}
	var in struct {
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if !s.verifyAccountPassword(c, user, in.Password) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) > 100 {
		oauthError(c, 400, "invalid_name")
		return
	}
	if in.Name == "" {
		in.Name = "Authenticator"
	}
	secret := credentials.NewSecret()
	encrypted, err := credentials.Seal(s.cfg.EncryptionKey, secret)
	if err != nil {
		s.fail(c, err)
		return
	}
	challenge := store.LoginChallenge{ID: rand.Text(), UserID: user.ID, Email: user.Email, Purpose: "enroll", Name: in.Name, PasswordHash: user.PasswordHash, SessionHash: digest(sessionID(c)), Secret: encrypted, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	uri := "otpauth://totp/" + url.PathEscape("AuthCenter:"+user.Email) + "?secret=" + secret + "&issuer=AuthCenter&algorithm=SHA1&digits=6&period=30"
	qrCode, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		s.fail(c, fmt.Errorf("encode authenticator QR code: %w", err))
		return
	}
	if err := s.store.CreateLoginChallenge(c.Request.Context(), challenge); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"request": challenge.ID, "secret": secret, "uri": uri, "qr_code": "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrCode)})
}

func (s *Server) verifyTOTPEnrollment(c *gin.Context) {
	user, _ := s.me(c)
	var in struct {
		Request string `json:"request"`
		Code    string `json:"code"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	challenge, err := s.store.LoginChallengeByID(c.Request.Context(), in.Request)
	if err != nil || challenge.UserID != user.ID || challenge.Purpose != "enroll" || challenge.SessionHash != digest(sessionID(c)) {
		oauthError(c, 401, "invalid_challenge")
		return
	}
	secret, err := credentials.Open(s.cfg.EncryptionKey, challenge.Secret)
	if err != nil {
		s.fail(c, fmt.Errorf("decrypt enrollment: %w", err))
		return
	}
	step, valid := credentials.VerifyTOTP(secret, in.Code, time.Now().UTC())
	if _, err := s.store.VerifyLoginChallenge(c.Request.Context(), challenge.ID, "", step, valid, 0); err != nil {
		oauthError(c, 401, "invalid_code")
		return
	}
	c.Status(http.StatusNoContent)
}
