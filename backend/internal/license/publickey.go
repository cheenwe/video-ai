package license

import (
	"bytes"
	_ "embed"
	"log"
	"os"
	"path/filepath"
)

//go:embed public.pem
var embeddedPublicKeyPEM []byte

const publicKeyFileName = "public.pem"

var localPublicKeyPath string

// InitPublicKey loads the license verification public key (verify-only; no private key in this repo).
// Priority: LICENSE_PUBLIC_KEY_PEM > LICENSE_PUBLIC_KEY_PATH >
// {dataHome}/public.pem (synced from embedded nextlicense public key).
func InitPublicKey(dataHome string) {
	if pemStr := os.Getenv("LICENSE_PUBLIC_KEY_PEM"); pemStr != "" {
		SetDefaultPublicKeyPEM([]byte(pemStr))
		log.Printf("license public key loaded from LICENSE_PUBLIC_KEY_PEM")
		return
	}
	if path := os.Getenv("LICENSE_PUBLIC_KEY_PATH"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warn: license public key %s: %v", path, err)
			return
		}
		localPublicKeyPath = path
		SetDefaultPublicKeyPEM(raw)
		log.Printf("license public key loaded from %s", path)
		return
	}

	embedded := bytes.TrimSpace(embeddedPublicKeyPEM)
	path := filepath.Join(dataHome, publicKeyFileName)
	if len(embedded) > 0 {
		existing, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			if err := os.WriteFile(path, embedded, 0o644); err != nil {
				log.Printf("warn: write license public key %s: %v", path, err)
			} else {
				log.Printf("license public key created at %s", path)
			}
		case err == nil && !bytes.Equal(bytes.TrimSpace(existing), embedded):
			if err := os.WriteFile(path, embedded, 0o644); err != nil {
				log.Printf("warn: update license public key %s: %v", path, err)
			} else {
				log.Printf("license public key updated at %s (synced from embedded)", path)
			}
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if len(embedded) > 0 {
			SetDefaultPublicKeyPEM(embedded)
			log.Printf("license public key loaded from embedded (data file unreadable)")
		} else {
			log.Printf("warn: license public key %s: %v", path, err)
		}
		return
	}
	localPublicKeyPath = path
	SetDefaultPublicKeyPEM(raw)
	log.Printf("license public key loaded from %s", path)
}

// LocalPublicKeyPath returns the resolved public.pem path under data home (if any).
func LocalPublicKeyPath() string {
	return localPublicKeyPath
}
