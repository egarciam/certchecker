package controller

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	"egarciam.com/checkcert/internal/email"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// emailShouldBeSent determines whether an email should be sent or resent
// based on the cooldown (to avoid flooding recipients). A mail should be sent
// when:
//   - it has not been sent before, or
//   - the last successful send has no valid timestamp, or
//   - the cooldown has elapsed.
//
// emailCooldownSeconds is expressed in seconds. A non-positive value means
// there is no cooldown.
func emailShouldBeSent(monitoredCert monitoringv1alpha1.MonitoredCertificateStatus, emailCooldownSeconds int, now time.Time) bool {
	if !monitoredCert.EmailSent {
		return true
	}

	// An empty or invalid timestamp means we cannot prove a recent send, so
	// send again rather than silently suppressing the alert.
	if monitoredCert.LastEmailSentAt == "" {
		return true
	}
	lastEmailSentAt, err := time.Parse(time.RFC3339, monitoredCert.LastEmailSentAt)
	if err != nil {
		return true
	}

	if emailCooldownSeconds <= 0 {
		return true
	}

	cooldownDue := lastEmailSentAt.Add(time.Duration(emailCooldownSeconds) * time.Second)
	return !now.Before(cooldownDue)
}

// findMonitoredCert returns the previous status for a certificate, or a zero
// value when the certificate has not been seen before.
func findMonitoredCert(statuses []monitoringv1alpha1.MonitoredCertificateStatus, name string) monitoringv1alpha1.MonitoredCertificateStatus {
	for _, status := range statuses {
		if status.Name == name {
			return status
		}
	}
	return monitoringv1alpha1.MonitoredCertificateStatus{}
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
// thresholdDays is the number of days before expiry at which a certificate is
// considered expiring.
func getCertificateStatus(expiry, now time.Time, thresholdDays int) string {
	if now.After(expiry) {
		return expired
	}
	if now.Add(time.Duration(thresholdDays) * 24 * time.Hour).After(expiry) {
		return expiring
	}
	return valid
}

const defaultRecipientsConfigMap = "email-recipients-config"

// getRecipients returns the configured email recipients. The ConfigMap is
// resolved from the CertificateMonitor spec, defaulting to the monitor's
// namespace and the standard ConfigMap name.
func (r *CertificateMonitorReconciler) getRecipients(ctx context.Context, certmonitor *monitoringv1alpha1.CertificateMonitor) ([]string, error) {
	log := log.FromContext(ctx)

	configMapName := certmonitor.Spec.EmailRecipientsConfigMap
	if configMapName == "" {
		configMapName = defaultRecipientsConfigMap
	}
	configMapNamespace := certmonitor.Spec.EmailRecipientsNamespace
	if configMapNamespace == "" {
		configMapNamespace = certmonitor.Namespace
	}

	// Fetch the ConfigMap with email recipients
	var configMap corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: configMapNamespace}, &configMap); err != nil {
		log.Error(err, "unable to fetch ConfigMap for email recipients",
			"name", configMapName, "namespace", configMapNamespace)
		return nil, err
	}

	var recipients []string
	if err := json.Unmarshal([]byte(configMap.Data["emails"]), &recipients); err != nil {
		log.Error(err, "unable to parse email recipients from ConfigMap",
			"name", configMapName, "namespace", configMapNamespace)
		return nil, err
	}
	return recipients, nil
}

// sendMail delegates to the email package. Tests inject MailSender instead.
func sendMail(subject, body, recipient string) error {
	return email.SendMail(subject, body, recipient)
}
