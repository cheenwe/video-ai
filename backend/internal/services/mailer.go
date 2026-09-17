package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"text/template"

	"baseplatform/internal/settings"
)

type Mailer struct{}

func (m *Mailer) Send(cfg settings.Email, to, subject, body string) error {
	if !cfg.Enabled {
		return fmt.Errorf("邮件服务未启用")
	}
	if cfg.SMTPHost == "" || cfg.From == "" {
		return fmt.Errorf("SMTP 配置不完整")
	}
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	msg := buildMessage(cfg.From, to, subject, body)
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
	if cfg.UseTLS {
		return sendTLS(addr, cfg.SMTPHost, auth, cfg.From, []string{to}, msg)
	}
	return smtp.SendMail(addr, auth, cfg.From, []string{to}, msg)
}

func buildMessage(from, to, subject, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(body)
	return buf.Bytes()
}

func sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func RenderTemplate(tpl string, data map[string]string) (string, error) {
	t, err := template.New("mail").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
