package license

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
)

// VerifyManifest 使用公钥 RSA-PSS 校验升级清单签名。
func VerifyManifest(raw []byte, signatureB64 string) error {
	pub, err := loadPublicKey()
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	return rsa.VerifyPSS(pub, crypto.SHA256, hash[:], sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto})
}
