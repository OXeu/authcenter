// Package oidc signs OpenID Connect ID Tokens and publishes their public keys.
package oidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

// JWK is the public portion of an RSA signing key.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// Claims contains the identity and authorization-code bindings in an ID Token.
type Claims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      string `json:"aud"`
	IssuedAt      int64  `json:"iat"`
	ExpiresAt     int64  `json:"exp"`
	Nonce         string `json:"nonce,omitempty"`
	AccessHash    string `json:"at_hash"`
	Name          string `json:"name,omitempty"`
	Picture       string `json:"picture,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified *bool  `json:"email_verified,omitempty"`
}

// Signer holds an active RSA key and public keys retained during rotation.
type Signer struct {
	key  *rsa.PrivateKey
	keys []JWK
}

// NewSigner loads a base64-encoded PEM bundle; the first key signs new tokens.
func NewSigner(encoded string) (*Signer, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("signing keys must be a base64-encoded PEM bundle")
	}
	signer := &Signer{}
	for strings.TrimSpace(string(raw)) != "" {
		if !strings.HasPrefix(strings.TrimSpace(string(raw)), "-----BEGIN ") || len(signer.keys) == 5 {
			return nil, errors.New("signing keys must contain one to five RSA private keys")
		}
		block, rest := pem.Decode(raw)
		if block == nil || len(block.Headers) != 0 {
			return nil, errors.New("invalid signing key PEM")
		}
		key, err := privateKey(block)
		if err != nil {
			return nil, err
		}
		public := x509.MarshalPKCS1PublicKey(&key.PublicKey)
		hash := sha256.Sum256(public)
		kid := base64.RawURLEncoding.EncodeToString(hash[:])
		for _, existing := range signer.keys {
			if existing.Kid == kid {
				return nil, errors.New("duplicate signing key")
			}
		}
		signer.keys = append(signer.keys, JWK{
			Kty: "RSA", Use: "sig", Alg: "RS256", Kid: kid,
			N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		})
		if signer.key == nil {
			signer.key = key
		}
		raw = rest
	}
	if signer.key == nil {
		return nil, errors.New("at least one signing key is required")
	}
	return signer, nil
}

func privateKey(block *pem.Block) (*rsa.PrivateKey, error) {
	var key *rsa.PrivateKey
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			var ok bool
			key, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.New("signing key must be RSA")
			}
		}
	default:
		return nil, errors.New("signing keys must use PKCS1 or PKCS8 private key PEM")
	}
	if err != nil {
		return nil, errors.New("invalid RSA signing key")
	}
	if key.N.BitLen() < 2048 || key.N.BitLen() > 8192 {
		return nil, errors.New("RSA signing keys must contain 2048 to 8192 bits")
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("invalid RSA signing key")
	}
	return key, nil
}

// Keys returns only public signing-key parameters.
func (s *Signer) Keys() []JWK { return slices.Clone(s.keys) }

// Sign creates an RS256 JWT using the active key.
func (s *Signer) Sign(claims Claims) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.keys[0].Kid})
	if err != nil {
		return "", fmt.Errorf("encode ID Token header: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode ID Token claims: %w", err)
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	hash := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign ID Token: %w", err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
