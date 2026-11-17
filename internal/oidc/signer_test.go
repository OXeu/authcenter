package oidc

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
)

func signingPEM(t *testing.T, bits int) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func verifySignature(t *testing.T, token string, keys []JWK) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("invalid JWT serialization")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(header, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["alg"] != "RS256" || fields["typ"] != "JWT" {
		t.Fatal("unexpected JWT header", fields)
	}
	for _, key := range keys {
		if key.Kid != fields["kid"] {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			t.Fatal(err)
		}
		e, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			t.Fatal(err)
		}
		public := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, hash[:], signature); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("JWT kid not published")
}

func TestSigningKeyPersistenceAndRotation(t *testing.T) {
	oldPEM, newPEM := signingPEM(t, 2048), signingPEM(t, 2048)
	old, err := NewSigner(base64.StdEncoding.EncodeToString(oldPEM))
	if err != nil {
		t.Fatal(err)
	}
	claims := Claims{Issuer: "https://auth.example", Subject: "42", Audience: "client", IssuedAt: 100, ExpiresAt: 400, Nonce: "nonce"}
	oldToken, err := old.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewSigner(base64.StdEncoding.EncodeToString(oldPEM))
	if err != nil {
		t.Fatal(err)
	}
	verifySignature(t, oldToken, restarted.Keys())
	rotated, err := NewSigner(base64.StdEncoding.EncodeToString(append(newPEM, oldPEM...)))
	if err != nil {
		t.Fatal(err)
	}
	verifySignature(t, oldToken, rotated.Keys())
	newToken, err := rotated.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	verifySignature(t, newToken, rotated.Keys()[:1])
	keys := rotated.Keys()
	if len(keys) != 2 || keys[0].Kid == old.Keys()[0].Kid || keys[1] != old.Keys()[0] {
		t.Fatal("rotation did not preserve the previous public key")
	}
	keys[0].N = "changed"
	verifySignature(t, newToken, rotated.Keys())
	raw, err := json.Marshal(rotated.Keys())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"d":`, `"p":`, `"q":`, "PRIVATE"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("JWKS exposed private key material")
		}
	}
}

func TestSigningKeyValidation(t *testing.T) {
	valid := signingPEM(t, 2048)
	block, _ := pem.Decode(valid)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSigner(base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))); err != nil {
		t.Fatal("PKCS8 not accepted", err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"empty": nil, "malformed": []byte("not a key"), "weak": signingPEM(t, 1024),
		"duplicate":     append(append([]byte{}, valid...), valid...),
		"trailing data": append(append([]byte{}, valid...), []byte("garbage")...),
		"non-RSA":       pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}),
		"public":        pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)}),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSigner(base64.StdEncoding.EncodeToString(raw)); err == nil {
				t.Fatal("invalid signing key accepted")
			}
		})
	}
	if _, err := NewSigner("sensitive invalid base64"); err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("invalid configuration accepted or exposed")
	}
}
