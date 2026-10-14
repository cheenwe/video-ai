package security

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// ValidatePublicHTTPURL rejects private/link-local/loopback targets (SSRF).
func ValidatePublicHTTPURL(raw string) error {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("URL 无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https")
	}
	host := u.Hostname()
	if host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("URL 无效")
	}
	if allowPrivateWebhook() {
		return nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("禁止指向本机或内网地址")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
			return fmt.Errorf("禁止指向本机或内网地址")
		}
		if ip := net.ParseIP(host); ip != nil {
			return nil
		}
		return nil
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("禁止指向本机或内网地址")
		}
	}
	return nil
}

// ResolveAndValidatePublicHTTPURL resolves host and rejects blocked IPs (call before HTTP).
func ResolveAndValidatePublicHTTPURL(raw string) error {
	if err := ValidatePublicHTTPURL(raw); err != nil {
		return err
	}
	if allowPrivateWebhook() {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("URL 无效")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("禁止指向本机或内网地址")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("无法解析 Webhook 主机")
	}
	if len(ips) == 0 {
		return fmt.Errorf("无法解析 Webhook 主机")
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("禁止指向本机或内网地址")
		}
	}
	return nil
}

func allowPrivateWebhook() bool {
	return os.Getenv("BASE_WEBHOOK_ALLOW_PRIVATE") == "1"
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}
