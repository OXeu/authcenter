// Package credentials implements password hashing, encrypted TOTP secrets and verification codes.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a bounded password using bcrypt.
func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", errors.New("password must contain 12–72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a password with its stored hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewSecret generates an authenticator secret.
func NewSecret() string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(rand.Text()))
}

// Seal encrypts an OTP secret with a persistent 256-bit application key.
func Seal(key []byte, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nil, nil, []byte(value), []byte("authcenter-totp"))), nil
}

// Open decrypts an OTP secret.
func Open(key []byte, value string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	plain, err := aead.Open(nil, nil, data, []byte("authcenter-totp"))
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plain), nil
}

// Code generates a six-digit random email verification code.
func Code() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate email code: %w", err)
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

// CodeHash authenticates a short code without allowing offline enumeration from a database copy.
func CodeHash(key []byte, id, code string) string {
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(id + ":" + code)); err != nil {
		panic(err)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// TOTP returns the RFC 6238 code for a time step.
func TOTP(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("decode TOTP: %w", err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	if _, err := mac.Write(counter[:]); err != nil {
		return "", fmt.Errorf("calculate TOTP: %w", err)
	}
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

// VerifyTOTP accepts a single adjacent time step and returns its replay counter.
func VerifyTOTP(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return 0, false
	}
	for delta := int64(-1); delta <= 1; delta++ {
		step := now.Unix()/30 + delta
		expected, err := TOTP(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
