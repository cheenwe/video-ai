package services

import (
	"bytes"
	"encoding/base64"
	"github.com/pquerna/otp"
	"image/png"
	"strings"

	"github.com/pquerna/otp/totp"
	"baseplatform/internal/security"
)

func GenerateTOTPSecret(issuer, account string) (secret string, otpURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		SecretSize:  20,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func VerifyTOTP(secret, code string) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}

func CreateTempToken(secret, algorithm string, userID uint, username, role string, minutes int) (string, error) {
	token, _, err := security.CreatePurposeToken(secret, algorithm, minutes, userID, username, role, "totp")
	return token, err
}

func QRCodeURL(otpURL string) string {
	key, err := otp.NewKeyFromURL(otpURL)
	if err != nil {
		return ""
	}
	img, err := key.Image(200, 200)
	if err != nil {
		return ""
	}
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
