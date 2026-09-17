package license

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

// defaultCryptoSalt 为兜底盐，与签发服务（nextlicense）保持一致，否则即使签名通过也无法解密授权内容。
// 可通过 LICENSE_SALT 环境变量覆盖，避免盐硬编码在二进制中被轻易提取。
const defaultCryptoSalt = "nextlicense-v1"

func licenseSalt() string {
	if s := os.Getenv("LICENSE_SALT"); s != "" {
		return s
	}
	return defaultCryptoSalt
}

func deriveKey(licenseID, machineID string) []byte {
	h := sha256.Sum256([]byte(licenseID + ":" + machineID + ":" + licenseSalt()))
	return h[:]
}

func decryptPayload(ciphertextB64, licenseID, machineID string) (*LicensePayload, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	key := deriveKey(licenseID, machineID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	var payload LicensePayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func loadPublicKey() (*rsa.PublicKey, error) {
	if path := os.Getenv("LICENSE_PUBLIC_KEY_PATH"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return parsePublicKey(raw)
	}
	if pemStr := os.Getenv("LICENSE_PUBLIC_KEY_PEM"); pemStr != "" {
		return parsePublicKey([]byte(pemStr))
	}
	return parsePublicKey(defaultPublicKeyPEM)
}

func parsePublicKey(raw []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not RSA public key")
	}
	return rsaPub, nil
}

func verifyAndDecrypt(lic *SignedLicense, licenseID, machineID string) (*LicensePayload, error) {
	pub, err := loadPublicKey()
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(lic.Signature)
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}
	hash := sha256.Sum256([]byte(lic.Payload))
	if err := rsa.VerifyPSS(pub, crypto.SHA256, hash[:], sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto}); err != nil {
		return nil, errors.New("signature verification failed")
	}
	lid := licenseID
	if lid == "" {
		lid = lic.LicenseID
	}
	mid := machineID
	if mid == "" {
		mid = "offline"
	}
	return decryptPayload(lic.Payload, lid, mid)
}

func decodeLicenseFile(raw []byte) (*SignedLicense, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New("empty license file")
	}
	if raw[0] != '{' {
		if dec, err := base64.StdEncoding.DecodeString(string(raw)); err == nil && len(dec) > 0 {
			raw = bytes.TrimSpace(dec)
		}
	}
	var lic SignedLicense
	if err := json.Unmarshal(raw, &lic); err != nil {
		return nil, err
	}
	if lic.Payload == "" || lic.Signature == "" {
		return nil, errors.New("incomplete license file")
	}
	return &lic, nil
}

func verifyCRL(payload string, signature string) error {
	pub, err := loadPublicKey()
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(payload))
	return rsa.VerifyPSS(pub, crypto.SHA256, hash[:], sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto})
}

func tryDecryptLicense(lic *SignedLicense) (*LicensePayload, error) {
	if _, err := loadPublicKey(); err != nil {
		return nil, errors.New("授权公钥未配置，请设置 LICENSE_PUBLIC_KEY_PATH 指向签发服务的 public.pem")
	}
	lid := lic.LicenseID
	var lastSigErr error
	for _, mid := range machineIDCandidates() {
		payload, err := verifyAndDecrypt(lic, lid, mid)
		if err == nil {
			return payload, nil
		}
		if strings.Contains(err.Error(), "signature") {
			lastSigErr = err
		}
	}
	if payload, err := verifyAndDecrypt(lic, lid, "offline"); err == nil {
		return payload, nil
	} else if strings.Contains(err.Error(), "signature") {
		lastSigErr = err
	}
	if lastSigErr != nil {
		return nil, errors.New("授权签名无效，请确认服务器公钥与签发服务一致")
	}
	return nil, errors.New("无法解密授权文件，请确认绑定指纹与本机一致，或改用 floating/在线激活")
}

// defaultPublicKeyPEM 与 nextfile/nextlicense 签发服务使用同一对 RSA 密钥：
// 私钥由 nextlicense 的 KeysDir（data/keys/private.pem）持有用于签发，
// 公钥内嵌于此用于验签，保证 nextfile 签发的授权可直接被本系统验证。
var defaultPublicKeyPEM = []byte(`-----BEGIN PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAqiZ1fFqa9k8WHUY87f//
Ec2E15KgAeSxpdD8e/pdVIjoitSWoX8VDlx5Ukuyc/KEOLNgiL06pA+qcVCmWVK9
mt/lJIELefMsmFYWKxgGZ/iVR/0RzBqeOoauQGhaOwsvdujxpoXGaJZZQScDHClb
fh6C9NgchrOFFQHw7ANFrL3mrhJ/uFfQYLKrF1sB4hJzZLEdzD5YKo+GpGoJmMpc
LgV+1mgRI6pQLN1qO27j0JvFq97/kF4PUSTjudP7fPcbIiliaZRiMPAjlkVNnsFD
T//pOGpN9tdXyD8yKpHjk7v2EoQ/e/Gh8YKs2l0Vfb0cF7rRHSarzgB5bk4A3UGX
xBmnAiPkSLXCTQvzMf/E2y5VHg2o4cvLV6/fyCGo0TChDgiibDSVxFmWe4w+9CQx
TtCirr/fXNft8knegaAb4tsHt5thGHErMyuzujJUZ3VdDusrI2m3LgIz6bWl6SIn
Fmsk4FPV2fUR2+mW8q0BTTOgubvwK8tYY3EsEoTxCWB9XQER31UezNrGd818njAm
TR3cVdsjaPLQ/fCg+nRwRUSktxHR3TQnpJMPVvBWhzt4lETfE+bM2gN/Q3EedvD+
WPsTEkID5gY2I7Gg/dPaDAagYXJa5c88AdYb36MQDZYo5lizoM85MsNwj8NHnbkk
4nesFPJQs6ZA0zgKEOWK1OkCAwEAAQ==
-----END PUBLIC KEY-----`)

func SetDefaultPublicKeyPEM(pemBytes []byte) {
	defaultPublicKeyPEM = pemBytes
}
