package email

import (
	"fmt"

	"gopkg.in/gomail.v2"
)

func SendMail(subject, body, recipient string) error {
	// Internal SMTP server details
	// smtpHost := "mailhog-service.default.svc.cluster.local" // Internal mail server service
	smtpHost := "mailhog-service"
	smtpPort := 1025 // SMTP port (MailHog default)

	// Create the email
	m := gomail.NewMessage()
	m.SetHeader("From", "no-reply@example.com") // Sender address
	m.SetHeader("To", recipient)                // Recipient address
	m.SetHeader("Subject", subject)             // Email subject
	m.SetBody("text/plain", body)               // Email body

	// Set up the SMTP server
	d := gomail.NewDialer(smtpHost, smtpPort, "", "") // No authentication required for MailHog

	// Send the email
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
