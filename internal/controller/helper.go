package controller

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	"gopkg.in/gomail.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Funcion para determinar si hay que enviar o reenviar un correo en funcion
// del cooldowntiming (para no saturar)
func emailShouldBeSent(monitoredCert monitoringv1alpha1.MonitoredCertificateStatus, emailCooldownHours time.Duration) bool {
	log := log.FromContext(context.Background())
	if !monitoredCert.EmailSent {
		return true
	}

	//el ultimo correo enviado no se puede obtener del status asi que enviamos por si acaso
	lastEmailSentAt, err := time.Parse(time.RFC3339, monitoredCert.LastEmailSentAt)
	if err != nil {
		// If parsing fails, assume email wasn't recently sent
		return true
	}

	// Check if the cooldown period has elapsed. En negativo, si el tiempo se ha excedido hay que enviar de nuevo
	result := time.Now().Before(lastEmailSentAt.Add(emailCooldownHours * time.Second))
	log.Info("emailShouldBeSent", "result", time.Now().Before(lastEmailSentAt.Add(emailCooldownHours*time.Second)), "cooldown due", lastEmailSentAt.Add(emailCooldownHours*time.Second))
	return result
}

func (r *CertificateMonitorReconciler) getInternalCertExpiry(ctx context.Context, namespace, secretName string) (time.Time, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, secret); err != nil {
		return time.Time{}, err
	}

	certData := secret.Data["tls.crt"]
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		return time.Time{}, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}

// getCertificateStatus determines if a certificate is valid, expiring, or expired.
func getCertificateStatus(expiry time.Time) string {
	now := time.Now()
	if now.After(expiry) {
		return expired
	}
	if now.Add(30 * 24 * time.Hour).After(expiry) {
		return expiring
	}
	return valid
}

// getExternalCertExpiry fetches the expiry date from an external HTTPS endpoint.
func (r *CertificateMonitorReconciler) getExternalCertExpiry(url string) (time.Time, error) {
	resp, err := http.Get(url)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	cert := resp.TLS.PeerCertificates[0]
	return cert.NotAfter, nil
}

// funcion para obtener la lista de recipients del config map
func (r *CertificateMonitorReconciler) getRecipients(ctx context.Context) ([]string, error) {
	// Parse email recipients
	log := log.FromContext(context.Background())
	// Fetch the ConfigMap with email recipients
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: r.ConfigMapName, Namespace: "default"}, &configMap); err != nil {
		log.Error(err, "unable to fetch ConfigMap for email recipients")
		return nil, err
	}

	var recipients []string
	if err := json.Unmarshal([]byte(configMap.Data["emails"]), &recipients); err != nil {
		log.Error(err, "unable to parse email recipients from ConfigMap")
		return nil, err
	}
	return recipients, nil
}

// func SendMail
func sendMail(subject, body, recipient string) error {
	// Internal SMTP server details
	// smtpHost := "mailhog-service.default.svc.cluster.local" // Internal mail server service
	smtpHost := "mailhog-service.default.svc.cluster.local"
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
