package server

import (
	"crypto/rand"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
)

func (s *Server) verifyAccountPassword(c *gin.Context, user store.User, password string) bool {
	if !s.rateLimit(c, "account", strconv.FormatUint(uint64(user.ID), 10), 10, 900) {
		return false
	}
	if (user.PasswordHash != "" && !credentials.CheckPassword(user.PasswordHash, password)) || (user.PasswordHash == "" && user.OAuthSub == "") {
		oauthError(c, 401, "invalid_credentials")
		return false
	}
	return true
}

func sessionID(c *gin.Context) string {
	session, err := c.Cookie("ac_session")
	if err != nil {
		return ""
	}
	return session
}

func (s *Server) setPassword(c *gin.Context) {
	if !s.cfg.LocalLogin {
		oauthError(c, 403, "local_login_disabled")
		return
	}
	var in struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	user, _ := s.me(c)
	if !s.verifyAccountPassword(c, user, in.CurrentPassword) {
		return
	}
	hash, err := credentials.HashPassword(in.Password)
	if err != nil {
		oauthError(c, 400, "password_length")
		return
	}
	if err := s.store.SetPassword(c.Request.Context(), user, hash, sessionID(c)); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(204)
}

func (s *Server) startEmailChange(c *gin.Context) {
	if s.cfg.ResendKey == "" || s.cfg.ResendFrom == "" || len(s.cfg.EncryptionKey) != 32 {
		oauthError(c, 503, "email_unavailable")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	user, _ := s.me(c)
	if !s.verifyAccountPassword(c, user, in.Password) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 || in.Email == user.Email {
		oauthError(c, 400, "invalid_email")
		return
	}
	if _, err := s.store.UserByEmail(c.Request.Context(), in.Email); err == nil {
		oauthError(c, 409, "email_in_use")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(c, err)
		return
	}
	challenge := store.LoginChallenge{ID: rand.Text(), UserID: user.ID, Email: in.Email, PreviousEmail: user.Email, PasswordHash: user.PasswordHash, SessionHash: digest(sessionID(c)), Purpose: "email_change", ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if err := s.store.CreateLoginChallenge(c.Request.Context(), challenge); err != nil {
		s.fail(c, err)
		return
	}
	if s.deliverChallenge(c, challenge) {
		c.JSON(200, gin.H{"request": challenge.ID})
	}
}

func (s *Server) verifyEmailChange(c *gin.Context) {
	var in struct {
		Request string `json:"request"`
		Code    string `json:"code"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	user, _ := s.me(c)
	err := s.store.VerifyEmailChange(c.Request.Context(), user.ID, in.Request, digest(sessionID(c)), credentials.CodeHash(s.cfg.EncryptionKey, in.Request, in.Code), sessionID(c))
	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		oauthError(c, 409, "email_in_use")
	case err != nil:
		oauthError(c, 401, "invalid_code")
	default:
		c.Status(204)
	}
}

func (s *Server) disableTOTP(c *gin.Context) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	user, _ := s.me(c)
	if !s.verifyAccountPassword(c, user, in.Password) {
		return
	}
	if user.OAuthSub == "" && (s.cfg.ResendKey == "" || s.cfg.ResendFrom == "") {
		oauthError(c, 400, "second_factor_unavailable")
		return
	}
	secret, err := credentials.Open(s.cfg.EncryptionKey, user.TOTPSecret)
	if err != nil {
		oauthError(c, 401, "invalid_code")
		return
	}
	step, valid := credentials.VerifyTOTP(secret, in.Code, time.Now().UTC())
	if !valid {
		oauthError(c, 401, "invalid_code")
		return
	}
	if err := s.store.DisableTOTP(c.Request.Context(), user, step, sessionID(c)); err != nil {
		oauthError(c, 401, "invalid_code")
		return
	}
	c.Status(204)
}
