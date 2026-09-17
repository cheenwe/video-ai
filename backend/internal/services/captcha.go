package services

import (
	"bytes"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"baseplatform/internal/settings"
	"github.com/mojocn/base64Captcha"
)

type captchaEntry struct {
	answer        string
	caseSensitive bool
	expireAt      time.Time
}

type captchaMemStore struct {
	mu    sync.Mutex
	items map[string]captchaEntry
}

func newCaptchaMemStore() *captchaMemStore {
	s := &captchaMemStore{items: make(map[string]captchaEntry)}
	go s.gcLoop()
	return s
}

func (s *captchaMemStore) set(id, answer string, ttl time.Duration, caseSensitive bool) {
	s.mu.Lock()
	s.items[id] = captchaEntry{answer: answer, caseSensitive: caseSensitive, expireAt: time.Now().Add(ttl)}
	s.mu.Unlock()
}

func (s *captchaMemStore) verify(id, answer string) bool {
	s.mu.Lock()
	entry, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	s.mu.Unlock()
	if !ok || time.Now().After(entry.expireAt) {
		return false
	}
	if entry.caseSensitive {
		return entry.answer == answer
	}
	return strings.EqualFold(entry.answer, answer)
}

func (s *captchaMemStore) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, entry := range s.items {
			if now.After(entry.expireAt) {
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
	}
}

type CaptchaStore struct {
	store *captchaMemStore
}

func NewCaptchaStore() *CaptchaStore {
	return &CaptchaStore{store: newCaptchaMemStore()}
}

type CaptchaChallenge struct {
	ID          string `json:"captcha_id"`
	ImageBase64 string `json:"image_base64"`
	Obfuscated  bool   `json:"obfuscated"`
}

func (s *CaptchaStore) Create(cfg settings.CaptchaConfig) (CaptchaChallenge, error) {
	cfg = settings.NormalizeCaptcha(cfg)
	driver := buildCaptchaDriver(cfg)
	id, question, answer := driver.GenerateIdQuestionAnswer()
	item, err := driver.DrawCaptcha(question)
	if err != nil {
		return CaptchaChallenge{}, err
	}
	s.store.set(id, answer, time.Duration(cfg.ExpirySeconds)*time.Second, cfg.CaseSensitive)
	var buf bytes.Buffer
	if _, err := item.WriteTo(&buf); err != nil {
		return CaptchaChallenge{}, err
	}
	raw := buf.Bytes()
	if cfg.ObfuscateImage {
		raw = xorObfuscate(id, raw)
	}
	return CaptchaChallenge{
		ID:          id,
		ImageBase64: base64.StdEncoding.EncodeToString(raw),
		Obfuscated:  cfg.ObfuscateImage,
	}, nil
}

func (s *CaptchaStore) Preview(cfg settings.CaptchaConfig) (string, error) {
	driver := buildCaptchaDriver(cfg)
	_, question, _ := driver.GenerateIdQuestionAnswer()
	item, err := driver.DrawCaptcha(question)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(item.EncodeB64string(), "data:image/png;base64,"), nil
}

func (s *CaptchaStore) Verify(id, answer string) bool {
	answer = strings.TrimSpace(answer)
	if id == "" || answer == "" {
		return false
	}
	return s.store.verify(id, answer)
}

func buildCaptchaDriver(cfg settings.CaptchaConfig) base64Captcha.Driver {
	cfg = settings.NormalizeCaptcha(cfg)
	switch cfg.Type {
	case "string":
		noise, lines := charNoiseOptions(cfg.NoiseLevel)
		source := base64Captcha.TxtNumbers + base64Captcha.TxtAlphabet
		return base64Captcha.NewDriverString(cfg.Height, cfg.Width, noise, lines, cfg.Length, source, nil, nil, nil).ConvertFonts()
	case "math":
		noise, lines := charNoiseOptions(cfg.NoiseLevel)
		return base64Captcha.NewDriverMath(cfg.Height, cfg.Width, noise, lines, nil, nil, nil).ConvertFonts()
	default:
		skew, dots := digitNoiseOptions(cfg.NoiseLevel)
		return base64Captcha.NewDriverDigit(cfg.Height, cfg.Width, cfg.Length, skew, dots)
	}
}

func digitNoiseOptions(level string) (maxSkew float64, dotCount int) {
	switch level {
	case "high":
		return 0.7, 80
	case "medium":
		return 0.45, 40
	default:
		return 0.2, 15
	}
}

func charNoiseOptions(level string) (noiseCount int, lineOptions int) {
	switch level {
	case "high":
		return 10, base64Captcha.OptionShowHollowLine | base64Captcha.OptionShowSlimeLine | base64Captcha.OptionShowSineLine
	case "medium":
		return 4, base64Captcha.OptionShowSlimeLine
	default:
		return 0, 0
	}
}

const captchaSalt = "va-cap-v1"

func fnv1a(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func xorObfuscate(id string, data []byte) []byte {
	x := fnv1a(captchaSalt + id)
	if x == 0 {
		x = 0x1a2b3c4d
	}
	out := make([]byte, len(data))
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = data[i] ^ byte(x)
	}
	return out
}
