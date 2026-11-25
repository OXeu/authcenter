package server

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
	"github.com/gin-gonic/gin"
)

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

var supportedScopes = []string{"openid", "profile", "email", "offline_access", mcpScope}

type clientInput struct {
	Name         string   `json:"name"`
	Homepage     string   `json:"homepage"`
	Logo         string   `json:"logo"`
	RedirectURIs []string `json:"redirect_uris"`
	Public       bool     `json:"public"`
}

func validAppURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Host == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Zone() == "" && ip.IsPrivate()
}

func clientFields(in clientInput, previous store.OAuthClient) (store.OAuthClient, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 || !validAppURL(in.Homepage) || len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 20 {
		return store.OAuthClient{}, fmt.Errorf("%w: name, homepage and 1–20 callback URLs are required", errBadRequest)
	}
	previous.Name, previous.Homepage, previous.RedirectURIs = strings.TrimSpace(in.Name), in.Homepage, slices.Clone(in.RedirectURIs)
	if in.Logo != "" {
		logo, err := base64.StdEncoding.DecodeString(in.Logo)
		if err != nil || len(logo) > 512*1024 {
			return store.OAuthClient{}, fmt.Errorf("%w: logo must be a PNG or JPEG up to 512 KiB", errBadRequest)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(logo))
		if err != nil || cfg.Width > 1024 || cfg.Height > 1024 || cfg.Width < 1 || cfg.Height < 1 || (format != "png" && format != "jpeg") {
			return store.OAuthClient{}, fmt.Errorf("%w: logo dimensions must be at most 1024 × 1024", errBadRequest)
		}
		if _, _, err := image.Decode(bytes.NewReader(logo)); err != nil {
			return store.OAuthClient{}, fmt.Errorf("%w: invalid logo image", errBadRequest)
		}
		previous.Logo, previous.LogoType = logo, "image/"+format
	}
	if len(previous.Logo) == 0 {
		return store.OAuthClient{}, fmt.Errorf("%w: upload an application logo", errBadRequest)
	}
	return previous, nil
}

func (s *Server) apiClients(c *gin.Context) {
	clients, err := s.store.ListOAuthClients(c.Request.Context())
	if err != nil {
		s.fail(c, err)
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	visible := []store.OAuthClient{}
	for _, client := range clients {
		if id.Role == store.RoleAdmin || client.OwnerID == id.User.ID {
			visible = append(visible, client)
		}
	}
	c.JSON(http.StatusOK, gin.H{"clients": visible})
}

func (s *Server) apiCreateClient(c *gin.Context) {
	var in clientInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	client, err := clientFields(in, store.OAuthClient{})
	if err != nil {
		s.fail(c, err)
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	client.CreatedAt = time.Now().UTC()
	client.ID, client.OwnerID, client.Public, client.Scopes = rand.Text(), id.User.ID, in.Public, supportedScopes
	secret := ""
	if !client.Public {
		secret = rand.Text() + rand.Text()
		client.SecretHash = digest(secret)
	}
	if err := s.store.CreateOAuthClient(c.Request.Context(), client); err != nil {
		s.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{"client": client, "client_secret": secret})
}

func (s *Server) ownedClient(c *gin.Context) (store.OAuthClient, bool) {
	client, err := s.store.OAuthClientByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.fail(c, err)
		return store.OAuthClient{}, false
	}
	id, _ := auth.FromContext(c.Request.Context())
	if id.Role != store.RoleAdmin && client.OwnerID != id.User.ID {
		c.Status(http.StatusForbidden)
		return store.OAuthClient{}, false
	}
	return client, true
}

func (s *Server) apiUpdateClient(c *gin.Context) {
	client, ok := s.ownedClient(c)
	if !ok {
		return
	}
	if client.Dynamic {
		c.Status(http.StatusForbidden)
		return
	}
	var in clientInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	client, err := clientFields(in, client)
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.UpdateOAuthClient(c.Request.Context(), client); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, client)
}

func (s *Server) apiDeleteClient(c *gin.Context) {
	client, ok := s.ownedClient(c)
	if !ok {
		return
	}
	if err := s.store.DisableOAuthClient(c.Request.Context(), client.ID); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) clientLogo(c *gin.Context) {
	client, err := s.store.OAuthClientByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, client.LogoType, client.Logo)
}

func (s *Server) apiGrants(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	grants, err := s.store.ListOAuthGrants(c.Request.Context(), id.User.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"grants": grants})
}

func (s *Server) apiRevokeGrant(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	if err := s.store.RevokeOAuthGrant(c.Request.Context(), id.User.ID, c.Param("id")); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
