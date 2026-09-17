package services

import (
	"crypto/tls"
	"fmt"
	"strings"

	"baseplatform/internal/settings"
	ldap "github.com/go-ldap/ldap/v3"
)

type LDAPAuth struct{}

type LDAPUser struct {
	Username string
	Email    string
	DN       string
}

func (l *LDAPAuth) dial(cfg settings.LDAP) (*ldap.Conn, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("请填写 Host")
	}
	port := cfg.Port
	if port == 0 {
		port = 389
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	var conn *ldap.Conn
	var err error
	if cfg.UseTLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = ldap.DialURL("ldap://" + addr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 LDAP 失败: %w", err)
	}
	return conn, nil
}

func (l *LDAPAuth) bindService(conn *ldap.Conn, cfg settings.LDAP) error {
	if cfg.BindDN == "" {
		return nil
	}
	if err := conn.Bind(cfg.BindDN, cfg.BindPass); err != nil {
		return fmt.Errorf("LDAP 绑定失败: %w", err)
	}
	return nil
}

func (l *LDAPAuth) Authenticate(cfg settings.LDAP, username, password string) (*LDAPUser, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("LDAP 未启用")
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("用户名或密码为空")
	}
	if !strings.Contains(cfg.UserFilter, "%s") {
		return nil, fmt.Errorf("用户过滤器须包含 %%s 占位符")
	}
	conn, err := l.dial(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := l.bindService(conn, cfg); err != nil {
		return nil, err
	}
	filter := fmt.Sprintf(cfg.UserFilter, ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		filter,
		[]string{"dn", cfg.UsernameAttr, cfg.EmailAttr},
		nil,
	)
	res, err := conn.Search(searchReq)
	if err != nil || len(res.Entries) == 0 {
		return nil, fmt.Errorf("LDAP 用户不存在")
	}
	entry := res.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return nil, fmt.Errorf("LDAP 认证失败")
	}
	uname := entry.GetAttributeValue(cfg.UsernameAttr)
	if uname == "" {
		uname = username
	}
	email := entry.GetAttributeValue(cfg.EmailAttr)
	return &LDAPUser{Username: uname, Email: email, DN: entry.DN}, nil
}

func (l *LDAPAuth) Test(cfg settings.LDAP) (string, error) {
	cfg = settings.MergeLDAPDefaults(cfg)
	if cfg.Host == "" || cfg.BaseDN == "" {
		return "", fmt.Errorf("请填写 Host 与 Base DN")
	}
	if cfg.UserFilter == "" || !strings.Contains(cfg.UserFilter, "%s") {
		return "", fmt.Errorf("用户过滤器须包含 %%s 占位符，例如 (uid=%%s)")
	}
	conn, err := l.dial(cfg)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := l.bindService(conn, cfg); err != nil {
		return "", err
	}
	var filter string
	if u := strings.TrimSpace(cfg.TestUsername); u != "" {
		filter = fmt.Sprintf(cfg.UserFilter, ldap.EscapeFilter(u))
	} else {
		filter = strings.Replace(cfg.UserFilter, "%s", "*", 1)
	}
	searchReq := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 5, 0, false,
		filter,
		[]string{"dn", cfg.UsernameAttr},
		nil,
	)
	res, err := conn.Search(searchReq)
	if err != nil {
		return "", fmt.Errorf("试搜失败（请检查 Base DN / 用户过滤器）: %w", err)
	}
	if strings.TrimSpace(cfg.TestUsername) != "" && len(res.Entries) == 0 {
		return "", fmt.Errorf("连接成功，但试搜用户 %q 无结果", cfg.TestUsername)
	}
	return fmt.Sprintf("LDAP 连接成功，试搜命中 %d 条", len(res.Entries)), nil
}

func LDAPUsernameFromDN(dn string) string {
	parts := strings.Split(dn, ",")
	if len(parts) == 0 {
		return dn
	}
	first := parts[0]
	if i := strings.Index(first, "="); i >= 0 {
		return first[i+1:]
	}
	return first
}
