package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"unicode"

	"baseplatform/internal/settings"
)

func ValidatePassword(pwd string, sec settings.Security) error {
	if len(pwd) > 72 {
		return fmt.Errorf("密码不得超过 72 字节")
	}
	if len(pwd) < sec.PasswordMinLen {
		return fmt.Errorf("密码至少 %d 位", sec.PasswordMinLen)
	}
	if sec.PasswordRequireUpper && !hasRune(pwd, unicode.IsUpper) {
		return fmt.Errorf("密码需包含大写字母")
	}
	if sec.PasswordRequireLower && !hasRune(pwd, unicode.IsLower) {
		return fmt.Errorf("密码需包含小写字母")
	}
	if sec.PasswordRequireDigit && !hasRune(pwd, unicode.IsDigit) {
		return fmt.Errorf("密码需包含数字")
	}
	if sec.PasswordRequireSpecial && !hasSpecial(pwd) {
		return fmt.Errorf("密码需包含特殊字符")
	}
	return nil
}

func hasRune(s string, fn func(rune) bool) bool {
	for _, r := range s {
		if fn(r) {
			return true
		}
	}
	return false
}

func hasSpecial(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func GeneratePassword(sec settings.Security) (string, error) {
	minLen := sec.PasswordMinLen
	if minLen < 10 {
		minLen = 10
	}
	lower := "abcdefghijklmnopqrstuvwxyz"
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits := "0123456789"
	special := "!@#$%&*"

	var pool string
	required := make([]byte, 0, 4)
	addRequired := func(chars string) error {
		c, err := randChar(chars)
		if err != nil {
			return err
		}
		required = append(required, c)
		return nil
	}
	if sec.PasswordRequireLower {
		if err := addRequired(lower); err != nil {
			return "", err
		}
		pool += lower
	}
	if sec.PasswordRequireUpper {
		if err := addRequired(upper); err != nil {
			return "", err
		}
		pool += upper
	}
	if sec.PasswordRequireDigit {
		if err := addRequired(digits); err != nil {
			return "", err
		}
		pool += digits
	}
	if sec.PasswordRequireSpecial {
		if err := addRequired(special); err != nil {
			return "", err
		}
		pool += special
	}
	if pool == "" {
		pool = lower + upper + digits
	}

	buf := make([]byte, 0, minLen)
	buf = append(buf, required...)
	for i := len(buf); i < minLen; i++ {
		c, err := randChar(pool)
		if err != nil {
			return "", err
		}
		buf = append(buf, c)
	}
	for i := len(buf) - 1; i > 0; i-- {
		j, err := randInt(i + 1)
		if err != nil {
			return "", err
		}
		buf[i], buf[j] = buf[j], buf[i]
	}
	pwd := string(buf)
	if err := ValidatePassword(pwd, sec); err != nil {
		return "", err
	}
	return pwd, nil
}

func randChar(chars string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
	if err != nil {
		return 0, err
	}
	return chars[n.Int64()], nil
}

func randInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func PasswordRulesText(sec settings.Security) []string {
	var rules []string
	rules = append(rules, fmt.Sprintf("至少 %d 位", sec.PasswordMinLen))
	if sec.PasswordRequireUpper {
		rules = append(rules, "包含大写字母")
	}
	if sec.PasswordRequireLower {
		rules = append(rules, "包含小写字母")
	}
	if sec.PasswordRequireDigit {
		rules = append(rules, "包含数字")
	}
	if sec.PasswordRequireSpecial {
		rules = append(rules, "包含特殊字符")
	}
	return rules
}

func MatchBlacklist(value string, patterns []string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(v, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
