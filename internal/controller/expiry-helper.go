package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getInternalCertExpiry fetches the expiry date from a Kubernetes secret.
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

// getCertificateStatus determines if a certificate is valid, expiring, or expired.
func GetCertificateStatus(expiry time.Time) string {
	now := time.Now()
	if now.After(expiry) {
		return expired
	}
	if now.Add(30 * 24 * time.Hour).After(expiry) {
		return expiring
	}
	return valid
}
