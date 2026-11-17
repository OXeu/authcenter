package server

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
)

func TestTOTPEnrollmentQRCode(t *testing.T) {
	for _, test := range []struct{ name, email string }{
		{"email-label", "scan+label@example.com"},
		{"long-email", strings.Repeat("u", 64) + "@" + strings.Repeat("d", 63) + "." + strings.Repeat("e", 63) + "." + strings.Repeat("f", 57) + ".com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, _ := localFlows(t)
			user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: test.email, Name: "Scanner", Role: store.RoleUser, OAuthSub: "scanner"})
			if err != nil {
				t.Fatal(err)
			}
			f = signedInUser(t, f, user)
			response := f.request("POST", "/auth/local/totp", map[string]string{"password": ""})
			if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("enrollment must return an uncached response", response.Code)
			}
			setup := responseJSON(t, response)
			encoded, ok := strings.CutPrefix(setup["qr_code"].(string), "data:image/png;base64,")
			if !ok {
				t.Fatal("QR image must be an inline PNG")
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			image, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if image.Bounds().Dx() != 256 || image.Bounds().Dy() != 256 {
				t.Fatal("unexpected QR image dimensions")
			}
			bitmap, err := gozxing.NewBinaryBitmapFromImage(image)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := qrcode.NewQRCodeReader().Decode(bitmap, nil)
			if err != nil {
				t.Fatal("QR image cannot be scanned", err)
			}
			if decoded.GetText() != setup["uri"] {
				t.Fatal("QR code differs from the provisioning URI")
			}
			uri, err := url.Parse(decoded.GetText())
			if err != nil {
				t.Fatal(err)
			}
			if uri.Scheme != "otpauth" || uri.Host != "totp" || uri.Path != "/AuthCenter:"+test.email {
				t.Fatal("QR code identifies the wrong account")
			}
			params := uri.Query()
			if params.Get("secret") != setup["secret"] || params.Get("issuer") != "AuthCenter" || params.Get("algorithm") != "SHA1" || params.Get("digits") != "6" || params.Get("period") != "30" {
				t.Fatal("QR code has incorrect authenticator settings")
			}
			before, err := f.srv.store.UserByID(t.Context(), user.ID)
			if err != nil || before.TOTPSecret != "" {
				t.Fatal("enrollment enabled before verification", err)
			}
			code, err := credentials.TOTP(params.Get("secret"), time.Now().Unix()/30)
			if err != nil {
				t.Fatal(err)
			}
			if verified := f.request("POST", "/auth/local/totp/verify", map[string]any{"request": setup["request"], "code": code}); verified.Code != 204 {
				t.Fatal("scanned secret cannot complete enrollment", verified.Code)
			}
		})
	}
}
