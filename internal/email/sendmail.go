package email

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/gomail.v2"
)

// Defaults used when neither the manager-wide defaults nor the
// CertificateMonitor spec override the SMTP settings. They match the MailHog
// service used by the sample deployment.
const (
	DefaultHost = "mailhog-service"
	DefaultPort = 1025
	DefaultFrom = "no-reply@example.com"
)

// Config describes how to reach the SMTP relay used for notifications.
type Config struct {
	// Host is the SMTP server hostname.
	Host string
	// Port is the SMTP server port (usually 25, 465 for TLS, or 1025 for
	// MailHog).
	Port int
	// From is the sender address used in the From header.
	From string
	// Username and Password are optional SMTP credentials. Authentication is
	// skipped when Username is empty.
	Username string
	Password string
	// TLS enables implicit TLS (SMTPS, usually port 465).
	TLS bool
}

// DefaultConfig returns the built-in SMTP configuration.
func DefaultConfig() Config {
	return Config{Host: DefaultHost, Port: DefaultPort, From: DefaultFrom}
}

// WithDefaults fills unset fields with the package defaults.
func (c Config) WithDefaults() Config {
	if c.Host == "" {
		c.Host = DefaultHost
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.From == "" {
		c.From = DefaultFrom
	}
	return c
}

// Validate reports whether the configuration can be used to send mail.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("smtp host is empty")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("smtp port %d is out of range 1-65535", c.Port)
	}
	if strings.TrimSpace(c.From) == "" {
		return errors.New("smtp sender address is empty")
	}
	if c.Password != "" && c.Username == "" {
		return errors.New("smtp password is set but username is empty")
	}
	return nil
}

// Send delivers a single plain-text message to one recipient.
func (c Config) Send(subject, body, recipient string) error {
	cfg := c.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", cfg.From)
	m.SetHeader("To", recipient)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(cfg.Host, cfg.Port, cfg.Username, cfg.Password)
	d.SSL = cfg.TLS

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
