package services

import (
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"baseplatform/internal/settings"
)

// AccountCaptchaAfterFails：同一账号连续密码失败达到该次数后，后续登录强制校验验证码。
const AccountCaptchaAfterFails = 3

type SecurityLoginLimiter struct {
	mu           sync.Mutex
	failures     map[string][]time.Time
	locked       map[string]time.Time
	accountFails map[string]int // 按用户名累计的连续失败次数
	db           *gorm.DB
}

func NewSecurityLoginLimiter(db *gorm.DB) *SecurityLoginLimiter {
	return &SecurityLoginLimiter{
		failures:     make(map[string][]time.Time),
		locked:       make(map[string]time.Time),
		accountFails: make(map[string]int),
		db:           db,
	}
}

func (l *SecurityLoginLimiter) sec() settings.Security {
	return settings.LoadSecurity(l.db)
}

func (l *SecurityLoginLimiter) AssertAllowed(ip string) (locked bool, waitSec int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until, ok := l.locked[ip]; ok && until.After(time.Now()) {
		return true, int(until.Sub(time.Now()).Seconds()) + 1
	}
	return false, 0
}

func (l *SecurityLoginLimiter) RecordFailure(ip string) bool {
	sec := l.sec()
	window := time.Duration(max(30, sec.LoginFailWindowSec)) * time.Second
	maxAttempts := max(1, sec.LoginFailMax)
	lockSec := time.Duration(max(60, sec.LoginBanSec)) * time.Second

	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var q []time.Time
	for _, t := range l.failures[ip] {
		if now.Sub(t) <= window {
			q = append(q, t)
		}
	}
	q = append(q, now)
	l.failures[ip] = q
	if len(q) >= maxAttempts {
		l.locked[ip] = now.Add(lockSec)
		l.failures[ip] = nil
		return true
	}
	return false
}

func (l *SecurityLoginLimiter) Clear(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
	delete(l.locked, ip)
}

func normalizeAccountKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// RecordAccountFailure 记录账号密码失败；返回当前是否已触发强制验证码。
func (l *SecurityLoginLimiter) RecordAccountFailure(username string) bool {
	key := normalizeAccountKey(username)
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.accountFails[key]++
	return l.accountFails[key] >= AccountCaptchaAfterFails
}

// RequireCaptcha 判断该账号是否因连续失败而必须校验验证码。
func (l *SecurityLoginLimiter) RequireCaptcha(username string) bool {
	key := normalizeAccountKey(username)
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.accountFails[key] >= AccountCaptchaAfterFails
}

// ClearAccount 登录成功后清零该账号的连续失败计数。
func (l *SecurityLoginLimiter) ClearAccount(username string) {
	key := normalizeAccountKey(username)
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.accountFails, key)
}

// IPRateLimiter 是基于滑动窗口的按 IP 请求限流器，用于保护公开接口
// （如验证码生成）免于被脚本高频刷取。
type IPRateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func NewIPRateLimiter(max int, window time.Duration) *IPRateLimiter {
	return &IPRateLimiter{
		hits:   make(map[string][]time.Time),
		max:    max,
		window: window,
	}
}

// Allow 记录一次来自 ip 的请求，若窗口内请求数未超过上限则返回 true。
func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var q []time.Time
	for _, t := range l.hits[ip] {
		if now.Sub(t) <= l.window {
			q = append(q, t)
		}
	}
	if len(q) >= l.max {
		l.hits[ip] = q
		return false
	}
	q = append(q, now)
	l.hits[ip] = q
	return true
}
