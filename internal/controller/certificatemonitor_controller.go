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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	// email "egarciam.com/checkcert/lib/email"
	"gopkg.in/gomail.v2"
)

// CertificateMonitorReconciler reconciles a CertificateMonitor object
type CertificateMonitorReconciler struct {
	client.Client
	ConfigMapName string // Name of the ConfigMap to fetch recipients
	Scheme        *runtime.Scheme
}

const (
	valid                string = "valid"
	expired              string = "expired"
	expiring             string = "expiring"
	defaultcheckinterval int    = 86400 //Valor por defecto.
)

//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch
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
	log := log.FromContext(context.Background())
	log.Info("EN RECONCILIATION LOOP")

	// TODO(user): your logic here
	certMonitor := &monitoringv1alpha1.CertificateMonitor{}
	if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
		log.Error(err, "unable to fetch CertificateMonitor")

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	updatedStatuses := []monitoringv1alpha1.MonitoredCertificateStatus{}

	if certMonitor.Spec.DiscoverInternal {
		log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados activada")
		internalCerts, err := r.discoverInternalCerts(ctx, certMonitor)
		if err != nil {
			log.Error(err, "failed to discover internal certs")
		} else {
			updatedStatuses = append(updatedStatuses, internalCerts...)
		}
		certMonitor.Status.MonitoredCertificates = updatedStatuses
		// log.Info(fmt.Sprintf("%v", updatedStatuses))
		if err := r.Status().Update(ctx, certMonitor); err != nil {
			log.Error(err, "failed to update CertificateMonitor status")
			return ctrl.Result{}, err
		}

	} else {
		log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados desactivada")
	}

	// for _, cert := range certMonitor.Spec.Certificates {
	// 	var status monitoringv1alpha1.MonitoredCertificateStatus
	// 	status.Name = cert.Name
	// 	if cert.Type == "internal" {
	// 		expiry, err := r.getInternalCertExpiry(ctx, cert.Namespace, cert.SecretName)
	// 		if err != nil {
	// 			log.Log.Error(err, "failed to get internal certificate expiry")
	// 			status.Status = "error"
	// 		} else {
	// 			status.Expiry = expiry.Format(time.RFC3339)
	// 			status.Status = getCertificateStatus(expiry)
	// 		}
	// 	} else if cert.Type == "external" {
	// 		expiry, err := r.getExternalCertExpiry(cert.URL)
	// 		if err != nil {
	// 			log.Log.Error(err, "failed to get external certificate expiry")
	// 			status.Status = "error"
	// 		} else {
	// 			status.Expiry = expiry.Format(time.RFC3339)
	// 			status.Status = getCertificateStatus(expiry)
	// 		}
	// 	}
	// 	updatedStatuses = append(updatedStatuses, status)
	// }

	// Update the status with retry logic
	// err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
	// 	// Fetch the latest version of the resource
	// 	if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
	// 		return err
	// 	}

	// 	// Update the status field
	// 	certMonitor.Status.MonitoredCertificates = updatedStatuses
	// 	return r.Status().Update(ctx, certMonitor)
	// })
	// if err != nil {
	// 	log.Log.Error(err, "failed to update CertificateMonitor status")
	// 	return ctrl.Result{}, err
	// }

	checkinterval := certMonitor.Spec.CheckInterval
	if checkinterval == 0 {
		checkinterval = defaultcheckinterval
	}
	log.Info("checkinterval", fmt.Sprintf("%v", checkinterval), "Periodo de actualizacion en segundos")
	//return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	return ctrl.Result{RequeueAfter: time.Duration(checkinterval) * time.Second}, nil

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
		return expired
	}
	if now.Add(30 * 24 * time.Hour).After(expiry) {
		return expiring
	}
	return valid
}

// logic to search for kubernetes.io/tls secrets across namespaces.
func (r *CertificateMonitorReconciler) discoverInternalCerts(ctx context.Context, certmonitor *monitoringv1alpha1.CertificateMonitor) ([]monitoringv1alpha1.MonitoredCertificateStatus, error) {
	var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus
	var recipients []string
	totals := struct {
		total    int
		expired  int
		expiring int
		valid    int
	}{0, 0, 0, 0}
	log := log.FromContext(context.Background())
	log.Info("discoverInternalCerts")
	// List all secrets of type kubernetes.io/tls
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.MatchingFields{"type": "kubernetes.io/tls"}); err != nil {
		log.Error(err, err.Error())
		return nil, err
	}

	if len(secretList.Items) == 0 {
		log.Info("discoverInternalCerts", "certificados:", len(secretList.Items), "No certs to review", "Exit")
		return certStatuses, nil
	}

	//Enviamos correo?
	if certmonitor.Spec.SendMail {
		log.Info("SendMail", fmt.Sprintf("%v", certmonitor.Spec.SendMail), "Envio de correo activo - habra que enviar correo con estado de los certificados")
		var err error
		recipients, err = r.getRecipients(ctx)
		if err != nil {
			log.Error(err, "Failed to get recipients")
		} else {
			log.Info("Recipients fetched successfully", "recipients", recipients)
		}
	}

	for _, secret := range secretList.Items {
		expiry, err := r.getInternalCertExpiry(ctx, secret.Namespace, secret.Name)
		if err != nil {
			log.Error(err, err.Error())
			continue // Handle error or log it
		}
		totals.total++

		switch getCertificateStatus(expiry) {
		case valid:
			totals.valid++
			log.Info("Valid certificate", "name", secret.Name, "expiry date", expiry.Format(time.RFC3339))
		case expiring:
			totals.expiring++
			log.Info("Expiring certificate", "name", secret.Name, "expiry date", expiry.Format(time.RFC3339), "days left", (expiry.Sub(time.Now())).Hours()/24)
			if certmonitor.Spec.SendMail {
				subject := fmt.Sprintf("Certificate Expiring: %s-%s", secret.Namespace, secret.Name)
				body := fmt.Sprintf("The certificate %s-%s is expiring on %s.", secret.Namespace, secret.Name, expiry.Format(time.RFC3339))
				if err := r.sendMails(subject, body, secret.Name, recipients); err != nil {
					log.Error(err, "failed to send email for expiring certificate", "name", secret.Name)
				} else {
					log.Info("Email sent for expiring certificate", "name", secret.Name)
				}
			}
		case expired:
			totals.expired++
			log.Info("Expired certificate", "name", secret.Name, "expiry date", expiry.Format(time.RFC3339))
			if certmonitor.Spec.SendMail {
				subject := fmt.Sprintf("Certificate Expired: %s", secret.Name)
				body := fmt.Sprintf("The certificate %s has expired on %s.", secret.Name, expiry.Format(time.RFC3339))
				if err := r.sendMails(subject, body, secret.Name, recipients); err != nil {
					log.Error(err, "failed to send email for expired certificate", "name", secret.Name)
				} else {
					log.Info("Email sent for expired certificate", "name", secret.Name)
				}
			}
		}

		certStatuses = append(certStatuses, monitoringv1alpha1.MonitoredCertificateStatus{
			Name:      fmt.Sprintf("internal-%s-%s", secret.Namespace, secret.Name),
			Status:    getCertificateStatus(expiry),
			Expiry:    expiry.Format(time.RFC3339),
			Namespace: secret.Namespace,
		})
	}
	log.Info("Cert Summary", "total", totals.total, "validos", totals.valid, "expirados", totals.expired, "expiring", totals.expiring)
	return certStatuses, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add an index for the Secret type
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &corev1.Secret{}, "type", func(o client.Object) []string {
		secret, ok := o.(*corev1.Secret)
		if !ok {
			return nil
		}
		return []string{string(secret.Type)}
	}); err != nil {
		return err
	}
	r.ConfigMapName = "email-recipients-config" // ConfigMap name with email recipients

	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.CertificateMonitor{}).
		Complete(r)
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

// funcion para enviar los correos a la lista de recipients
func (r *CertificateMonitorReconciler) sendMails(subject, body, name string, recipients []string) error {
	log := log.FromContext(context.Background())
	for _, recipient := range recipients {
		// Send the email
		// if err := email.SendMail(subject, body, recipient); err != nil {
		if err := SendMail(subject, body, recipient); err != nil {
			log.Error(err, "failed to send email", "recipient", recipient)
			return err
		} else {
			log.Info("Email sent successfully", "recipient", recipient, "certificate", name)
		}
	}
	return nil
}

func SendMail(subject, body, recipient string) error {
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
