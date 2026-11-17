package credentials

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestTOTPStandardVectorAndWindow(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	code, err := TOTP(secret, 59/30)
	if err != nil || code != "287082" {
		t.Fatalf("RFC 6238 vector: %s %v", code, err)
	}
	if _, ok := VerifyTOTP(secret, code, time.Unix(59, 0)); !ok {
		t.Fatal("valid OTP rejected")
	}
	if _, ok := VerifyTOTP(secret, code, time.Unix(180, 0)); ok {
		t.Fatal("expired OTP accepted")
	}
}

func TestCredentialEncryptionAuthenticatesKeyAndCiphertext(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	secret := NewSecret()
	sealed, err := Seal(key, secret)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Open(key, sealed)
	if err != nil || restored != secret {
		t.Fatal("secret roundtrip failed")
	}
	if _, err := Open([]byte(strings.Repeat("x", 32)), sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	if CodeHash(key, "one", "123456") == CodeHash(key, "two", "123456") {
		t.Fatal("email code not bound to challenge")
	}
}
