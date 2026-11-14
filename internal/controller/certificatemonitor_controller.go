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
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	// email "egarciam.com/checkcert/lib/email"
)

// CertificateMonitorReconciler reconciles a CertificateMonitor object
type CertificateMonitorReconciler struct {
	client.Client
	ConfigMapName string // Name of the ConfigMap to fetch recipients
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	// Now returns the current time. It is overridable for tests.
	Now func() time.Time
	// MailSender sends an email. It is overridable for tests.
	MailSender func(subject, body, recipient string) error
}

// now returns the reconciler clock, falling back to time.Now.
func (r *CertificateMonitorReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

const (
	valid                string = "valid"
	expired              string = "expired"
	expiring             string = "expiring"
	defaultcheckinterval int    = 86400 //Valor por defecto.
)

//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
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

	certMonitor := &monitoringv1alpha1.CertificateMonitor{}
	if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
		log.Error(err, "unable to fetch CertificateMonitor")

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	//Si no esta especificada la busqueda salimos y esperamos a que se actualice el spec
	if !certMonitor.Spec.DiscoverInternal {
		log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados NO activada.Modifique el objeto para iniciar la reconciliacion")
		return ctrl.Result{}, nil
	}

	log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados activada")
	updatedStatuses := []monitoringv1alpha1.MonitoredCertificateStatus{}
	internalCertsStatus, err := r.discoverInternalCerts(ctx, certMonitor)
	if err != nil {
		log.Error(err, "failed to discover internal certs")
		return ctrl.Result{}, err
	}
	updatedStatuses = append(updatedStatuses, internalCertsStatus...)
	certMonitor.Status.MonitoredCertificates = updatedStatuses

	// Update the status with retry logic
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the latest version of the resource
		if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
			return err
		}

		// Update the status field
		certMonitor.Status.MonitoredCertificates = updatedStatuses
		return r.Status().Update(ctx, certMonitor)
	})
	if err != nil {
		log.Error(err, "failed to update CertificateMonitor status")
		return ctrl.Result{}, err
	}

	checkinterval := certMonitor.Spec.CheckInterval
	if checkinterval == 0 {
		checkinterval = defaultcheckinterval
	}
	log.Info("checkinterval", fmt.Sprintf("%v", checkinterval), "Periodo de actualizacion en segundos")

	return ctrl.Result{RequeueAfter: time.Duration(checkinterval) * time.Second}, nil
}

// discoverInternalCerts searches for kubernetes.io/tls secrets across namespaces
// and returns the status of the certificates that need attention.
func (r *CertificateMonitorReconciler) discoverInternalCerts(ctx context.Context, certmonitor *monitoringv1alpha1.CertificateMonitor) ([]monitoringv1alpha1.MonitoredCertificateStatus, error) {
	var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus
	var recipients []string
	totals := struct {
		total    int
		expired  int
		expiring int
		valid    int
	}{0, 0, 0, 0}
	log := log.FromContext(ctx)
	now := r.now()
	log.Info("discoverInternalCerts")

	// List all secrets of type kubernetes.io/tls
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.MatchingFields{"type": "kubernetes.io/tls"}); err != nil {
		log.Error(err, err.Error())
		return nil, err
	}
	// si no hay secrets coincidentes salimos
	if len(secretList.Items) == 0 {
		log.Info("discoverInternalCerts", "certificados:", len(secretList.Items), "No certs to review", "Exit")
		return certStatuses, nil
	}

	// Enviamos correo?
	if certmonitor.Spec.SendMail {
		log.Info("SendMail", fmt.Sprintf("%v", certmonitor.Spec.SendMail), "Envio de correo activo - habra que enviar correo con estado de los certificados")
		var err error
		recipients, err = r.getRecipients(ctx)
		if err != nil {
			// Do not abort discovery: status is still useful even when mail is
			// misconfigured. The event makes the configuration problem visible.
			log.Error(err, "failed to get recipients; continuing without sending email")
			r.recordEvent(certmonitor, corev1.EventTypeWarning, "RecipientsUnavailable", "unable to load email recipients: "+err.Error())
			recipients = nil
		} else {
			log.Info("Recipients fetched successfully", "recipients", recipients)
		}
	}
	mailReady := certmonitor.Spec.SendMail && len(recipients) > 0

	// Secret loop
	for _, secret := range secretList.Items {
		expiry, err := r.getInternalCertExpiry(ctx, secret.Namespace, secret.Name)
		if err != nil {
			log.Error(err, err.Error())
			continue // Handle error or log it
		}
		totals.total++

		status := getCertificateStatus(expiry, now)
		var certStatus string
		switch status {
		case valid:
			totals.valid++
			certStatus = "Valid certificate"
			continue // Pasamos a procesar el siguiente certificado
		case expiring:
			totals.expiring++
			certStatus = "Expiring certificate"
		case expired:
			totals.expired++
			certStatus = "Expired certificate"
		}
		// mostramos log
		log.Info(certStatus, "name", secret.Namespace+"."+secret.Name, "expiry date", expiry.Format(time.RFC3339))

		// Si llegamos aqui son certificados chungos.
		// The previous status is only used for email bookkeeping. Expiry and
		// status are always recalculated so the status never goes stale.
		name := fmt.Sprintf("internal-%s.%s", secret.Namespace, secret.Name)
		previous := findMonitoredCert(certmonitor.Status.MonitoredCertificates, name)
		shouldSendMail := mailReady && emailShouldBeSent(previous, certmonitor.Spec.EmailCoolDown, now)

		certStatusEntry := monitoringv1alpha1.MonitoredCertificateStatus{
			Name:      name,
			Status:    status,
			Expiry:    expiry.Format(time.RFC3339),
			Namespace: secret.Namespace,
		}
		if certmonitor.Spec.SendMail {
			certStatusEntry.EmailSent = previous.EmailSent
			certStatusEntry.LastEmailSentAt = previous.LastEmailSentAt
		}

		// Send mail only when configured and cooldown has elapsed.
		if shouldSendMail {
			if err := r.sendMails(expiry, status, &secret, &recipients); err != nil {
				log.Error(err, "failed to send email for certificate", "name", name)
				r.recordEvent(certmonitor, corev1.EventTypeWarning, "EmailSendFailed",
					fmt.Sprintf("failed to send email for %s: %v", name, err))
			} else {
				certStatusEntry.EmailSent = true
				certStatusEntry.LastEmailSentAt = now.Format(time.RFC3339)
			}
		}

		certStatuses = append(certStatuses, certStatusEntry)
	}
	log.Info("Cert Summary", "total", totals.total, "validos", totals.valid, "expirados", totals.expired, "expiring", totals.expiring)
	return certStatuses, nil
}

// recordEvent emits a Kubernetes event when an event recorder is configured.
func (r *CertificateMonitorReconciler) recordEvent(obj runtime.Object, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
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
	r.Recorder = mgr.GetEventRecorderFor("certificatemonitor-controller")

	// Predicate to ignore status updates
	statusUpdatePredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Only reconcile if the spec or metadata changes
			oldResource := e.ObjectOld.DeepCopyObject().(*monitoringv1alpha1.CertificateMonitor)
			newResource := e.ObjectNew.DeepCopyObject().(*monitoringv1alpha1.CertificateMonitor)
			return oldResource.Generation != newResource.Generation
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.CertificateMonitor{},
			builder.WithPredicates(statusUpdatePredicate)).
		Complete(r)
}

// sendMails sends one email per recipient for a certificate that needs attention.
func (r *CertificateMonitorReconciler) sendMails(expiry time.Time, status string, secret *corev1.Secret, recipients *[]string) error {
	log := log.FromContext(context.Background())
	if recipients == nil || len(*recipients) == 0 {
		return fmt.Errorf("no email recipients configured")
	}

	var subject, body string
	totals := struct {
		expiring int
		expired  int
		total    int
	}{0, 0, 0}

	// Prepare subject and body of mails
	switch status {
	case expiring:
		subject = fmt.Sprintf("Certificate Expiring in %v days: %s.%s", math.Round(time.Until(expiry).Hours()/24), secret.Namespace, secret.Name)
		body = fmt.Sprintf("The certificate %s.%s is expiring on %s. %v days left", secret.Namespace, secret.Name, expiry.Format(time.RFC3339), math.Round(time.Until(expiry).Hours()/24))
	case expired:
		subject = fmt.Sprintf("Certificate Expired: %s.%s", secret.Namespace, secret.Name)
		body = fmt.Sprintf("The certificate %s.%s has expired on %s.", secret.Namespace, secret.Name, expiry.Format(time.RFC3339))
	}

	// Send mails
	for _, recipient := range *recipients {
		switch status {
		case expiring:
			totals.expiring++
		case expired:
			totals.expired++
		}
		totals.total++
		// Send the email
		if err := r.sendMail(subject, body, recipient); err != nil {
			log.Error(err, "failed to send email", "recipient", recipient)
			return err
		}
		log.Info("Email sent successfully", "recipient", recipient, "certificate", secret.Name, "namespace", secret.Namespace)
	}
	log.Info("Sent email figures", "certificate", secret.Name, "namespace",
		secret.Namespace, "expiring", totals.expiring, "expired", totals.expired, "total", totals.total)
	return nil
}

// sendMail uses the injected sender when present, otherwise the package default.
func (r *CertificateMonitorReconciler) sendMail(subject, body, recipient string) error {
	if r.MailSender != nil {
		return r.MailSender(subject, body, recipient)
	}
	return sendMail(subject, body, recipient)
}
