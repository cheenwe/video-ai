package validation

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._\-]+$`)

// EmailOK reports whether s is a reasonable email address.
func EmailOK(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 128 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	// reject bare local names without domain
	at := strings.LastIndex(addr.Address, "@")
	if at <= 0 || at == len(addr.Address)-1 {
		return false
	}
	domain := addr.Address[at+1:]
	return strings.Contains(domain, ".")
}

// UsernameOK checks length and allowed characters.
func UsernameOK(s string) error {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 3 {
		return fmt.Errorf("用户名至少 3 个字符")
	}
	if n > 64 {
		return fmt.Errorf("用户名过长")
	}
	if !usernamePattern.MatchString(s) {
		return fmt.Errorf("用户名仅允许字母、数字、点、下划线、短横线")
	}
	return nil
}

// BindMessage maps gin/validator bind errors for user create/update fields.
func BindMessage(err error, fields map[string]map[string]string) string {
	if err == nil {
		return "参数错误"
	}
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		for _, fe := range ve {
			if m, ok := fields[fe.Field()]; ok {
				if msg, ok := m[fe.Tag()]; ok {
					return msg
				}
				if msg, ok := m["*"]; ok {
					return msg
				}
			}
		}
	}
	return "参数错误"
}

// UserCreateBindMessage is the default mapping for admin user create.
func UserCreateBindMessage(err error) string {
	return BindMessage(err, map[string]map[string]string{
		"Username": {
			"required": "请填写用户名",
			"min":      "用户名至少 3 个字符",
			"max":      "用户名过长",
		},
		"Email": {
			"required": "请填写邮箱",
			"email":    "邮箱格式不正确",
			"max":      "邮箱过长",
		},
		"Password": {
			"required": "请填写密码",
			"min":      "密码至少 5 位",
			"max":      "密码过长",
		},
	})
}
