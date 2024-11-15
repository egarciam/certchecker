/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
)

// CertificateMonitorReconciler reconciles a CertificateMonitor object
type CertificateMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CertificateMonitor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *CertificateMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	certMonitor := &monitoringv1alpha1.CertificateMonitor{}
	if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
		log.Log.Error(err, "unable to fetch CertificateMonitor")

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	updatedStatuses := []monitoringv1alpha1.MonitoredCertificateStatus{}

	for _, cert := range certMonitor.Spec.Certificates {
		var status monitoringv1alpha1.MonitoredCertificateStatus
		status.Name = cert.Name
		if cert.Type == "internal" {
			expiry, err := r.getInternalCertExpiry(ctx, cert.Namespace, cert.SecretName)
			if err != nil {
				log.Log.Error(err, "failed to get internal certificate expiry")
				status.Status = "error"
			} else {
				status.Expiry = expiry.Format(time.RFC3339)
				status.Status = getCertificateStatus(expiry)
			}
		} else if cert.Type == "external" {
			expiry, err := r.getExternalCertExpiry(cert.URL)
			if err != nil {
				log.Log.Error(err, "failed to get external certificate expiry")
				status.Status = "error"
			} else {
				status.Expiry = expiry.Format(time.RFC3339)
				status.Status = getCertificateStatus(expiry)
			}
		}
		updatedStatuses = append(updatedStatuses, status)
	}

	certMonitor.Status.MonitoredCertificates = updatedStatuses
	if err := r.Status().Update(ctx, certMonitor); err != nil {
		log.Log.Error(err, "failed to update CertificateMonitor status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 24 * time.Hour}, nil
}

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
func getCertificateStatus(expiry time.Time) string {
	now := time.Now()
	if now.After(expiry) {
		return "expired"
	}
	if now.Add(30 * 24 * time.Hour).After(expiry) {
		return "expiring"
	}
	return "valid"
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.CertificateMonitor{}).
		Complete(r)
}
