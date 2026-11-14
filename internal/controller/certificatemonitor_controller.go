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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
)

// CertificateMonitorReconciler reconciles a CertificateMonitor object
type CertificateMonitorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
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

//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors,verbs=get;list;watch;create;update;patch;delete
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
	log := log.FromContext(ctx)
	log.Info("EN RECONCILIATION LOOP")
	start := time.Now()
	defer func() {
		reconcileDuration.Observe(time.Since(start).Seconds())
	}()

	certMonitor := &monitoringv1alpha1.CertificateMonitor{}
	if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
		log.Error(err, "unable to fetch CertificateMonitor")

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Si no esta especificada la busqueda salimos y esperamos a que se actualice el spec
	if !certMonitor.Spec.DiscoverInternal {
		log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados NO activada.Modifique el objeto para iniciar la reconciliacion")
		return ctrl.Result{}, nil
	}

	log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "verificación de certificados activada")
	now := r.now()
	updatedStatuses, summary, mailReady, err := r.discoverInternalCerts(ctx, certMonitor)
	if err != nil {
		reconcileErrors.Inc()
		log.Error(err, "failed to discover internal certs")
		meta.SetStatusCondition(&certMonitor.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DiscoveryFailed",
			Message:            err.Error(),
			ObservedGeneration: certMonitor.Generation,
		})
		if updateErr := r.Status().Update(ctx, certMonitor); updateErr != nil {
			log.Error(updateErr, "failed to update CertificateMonitor conditions")
		}
		return ctrl.Result{}, err
	}

	// Update the status with retry logic
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the latest version of the resource
		if err := r.Get(ctx, req.NamespacedName, certMonitor); err != nil {
			return err
		}

		certMonitor.Status.MonitoredCertificates = updatedStatuses
		certMonitor.Status.Summary = summary
		certMonitor.Status.LastChecked = now.Format(time.RFC3339)
		certMonitor.Status.ObservedGeneration = certMonitor.Generation

		meta.SetStatusCondition(&certMonitor.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			Message:            "Certificate discovery completed",
			ObservedGeneration: certMonitor.Generation,
		})
		if summary.Errors > 0 {
			meta.SetStatusCondition(&certMonitor.Status.Conditions, metav1.Condition{
				Type:               "Degraded",
				Status:             metav1.ConditionTrue,
				Reason:             "CertificateErrors",
				Message:            fmt.Sprintf("%d certificate(s) could not be parsed", summary.Errors),
				ObservedGeneration: certMonitor.Generation,
			})
		} else {
			meta.SetStatusCondition(&certMonitor.Status.Conditions, metav1.Condition{
				Type:               "Degraded",
				Status:             metav1.ConditionFalse,
				Reason:             "NoErrors",
				Message:            "All discovered certificates were parsed",
				ObservedGeneration: certMonitor.Generation,
			})
		}
		if certMonitor.Spec.SendMail {
			mailCondition := metav1.Condition{
				Type:               "MailConfigured",
				Status:             metav1.ConditionTrue,
				Reason:             "RecipientsLoaded",
				Message:            "Email recipients are configured",
				ObservedGeneration: certMonitor.Generation,
			}
			if !mailReady {
				mailCondition.Status = metav1.ConditionFalse
				mailCondition.Reason = "RecipientsUnavailable"
				mailCondition.Message = "Email recipients could not be loaded"
			}
			meta.SetStatusCondition(&certMonitor.Status.Conditions, mailCondition)
		} else {
			meta.RemoveStatusCondition(&certMonitor.Status.Conditions, "MailConfigured")
		}

		return r.Status().Update(ctx, certMonitor)
	})
	if err != nil {
		reconcileErrors.Inc()
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
// and returns the status of the certificates that need attention, aggregate
// counts, and whether email is ready to send.
func (r *CertificateMonitorReconciler) discoverInternalCerts(ctx context.Context, certmonitor *monitoringv1alpha1.CertificateMonitor) ([]monitoringv1alpha1.MonitoredCertificateStatus, monitoringv1alpha1.CertificateMonitorSummary, bool, error) {
	var recipients []string
	totals := struct {
		total    int
		expired  int
		expiring int
		valid    int
		errors   int
	}{}
	summary := monitoringv1alpha1.CertificateMonitorSummary{}
	log := log.FromContext(ctx)
	now := r.now()
	certificateExpiry.Reset()
	log.Info("discoverInternalCerts")

	// List all secrets of type kubernetes.io/tls
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.MatchingFields{"type": "kubernetes.io/tls"}); err != nil {
		log.Error(err, err.Error())
		return nil, summary, false, err
	}
	certStatuses := make([]monitoringv1alpha1.MonitoredCertificateStatus, 0, len(secretList.Items))

	// Enviamos correo?
	if certmonitor.Spec.SendMail {
		log.Info("SendMail", fmt.Sprintf("%v", certmonitor.Spec.SendMail), "Envio de correo activo - habra que enviar correo con estado de los certificados")
		var err error
		recipients, err = r.getRecipients(ctx, certmonitor)
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

	// si no hay secrets coincidentes salimos
	if len(secretList.Items) == 0 {
		log.Info("discoverInternalCerts", "certificados:", len(secretList.Items), "No certs to review", "Exit")
		return certStatuses, summary, mailReady, nil
	}

	// Secret loop
	for _, secret := range secretList.Items {
		expiry, err := r.getInternalCertExpiry(ctx, secret.Namespace, secret.Name)
		if err != nil {
			log.Error(err, err.Error())
			totals.errors++
			continue // Handle error or log it
		}
		totals.total++
		certificateExpiry.WithLabelValues(secret.Namespace, secret.Name).Set(float64(expiry.Unix()))

		status := getCertificateStatus(expiry, now, certmonitor.Spec.ExpiryThresholdDays)
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
		shouldSendMail := mailReady && emailShouldBeSent(previous, certmonitor.Spec.EmailCooldown, now)

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
			if err := r.sendMails(ctx, expiry, status, &secret, &recipients); err != nil {
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
	summary = monitoringv1alpha1.CertificateMonitorSummary{
		Total:    totals.total,
		Valid:    totals.valid,
		Expiring: totals.expiring,
		Expired:  totals.expired,
		Errors:   totals.errors,
	}
	certificatesByStatus.WithLabelValues(valid).Set(float64(summary.Valid))
	certificatesByStatus.WithLabelValues(expiring).Set(float64(summary.Expiring))
	certificatesByStatus.WithLabelValues(expired).Set(float64(summary.Expired))
	log.Info("Cert Summary", "total", summary.Total, "validos", summary.Valid, "expirados", summary.Expired, "expiring", summary.Expiring, "errors", summary.Errors)
	return certStatuses, summary, mailReady, nil
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
	r.Recorder = mgr.GetEventRecorderFor("certificatemonitor-controller")

	// Predicate to ignore status updates. Generation only changes when the
	// spec changes, so metadata-only updates are ignored too.
	statusUpdatePredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldResource := e.ObjectOld.DeepCopyObject().(*monitoringv1alpha1.CertificateMonitor)
			newResource := e.ObjectNew.DeepCopyObject().(*monitoringv1alpha1.CertificateMonitor)
			return oldResource.Generation != newResource.Generation
		},
	}

	isTLSSecret := func(obj client.Object) bool {
		secret, ok := obj.(*corev1.Secret)
		return ok && secret.Type == corev1.SecretTypeTLS
	}
	tlsSecretPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isTLSSecret(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return isTLSSecret(e.ObjectOld) || isTLSSecret(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isTLSSecret(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isTLSSecret(e.Object) },
	}

	hasRecipients := func(obj client.Object) bool {
		configMap, ok := obj.(*corev1.ConfigMap)
		if !ok {
			return false
		}
		_, ok = configMap.Data["emails"]
		return ok
	}
	recipientsConfigMapPredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return hasRecipients(e.Object) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return hasRecipients(e.ObjectOld) || hasRecipients(e.ObjectNew) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasRecipients(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return hasRecipients(e.Object) },
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.CertificateMonitor{},
			builder.WithPredicates(statusUpdatePredicate)).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToMonitors),
			builder.WithPredicates(tlsSecretPredicate)).
		Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapRecipientsConfigMapToMonitors),
			builder.WithPredicates(recipientsConfigMapPredicate)).
		Complete(r)
}

// mapSecretToMonitors enqueues every CertificateMonitor when a TLS Secret
// changes. Discovery is cluster-wide, so every monitor is affected.
func (r *CertificateMonitorReconciler) mapSecretToMonitors(ctx context.Context, _ client.Object) []reconcile.Request {
	return r.allMonitorRequests(ctx)
}

// mapRecipientsConfigMapToMonitors enqueues only the monitors that use the
// changed recipients ConfigMap.
func (r *CertificateMonitorReconciler) mapRecipientsConfigMapToMonitors(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	var monitors monitoringv1alpha1.CertificateMonitorList
	if err := r.List(ctx, &monitors); err != nil {
		log.FromContext(ctx).Error(err, "unable to list CertificateMonitors")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(monitors.Items))
	for _, monitor := range monitors.Items {
		name := monitor.Spec.EmailRecipientsConfigMap
		if name == "" {
			name = defaultRecipientsConfigMap
		}
		namespace := monitor.Spec.EmailRecipientsNamespace
		if namespace == "" {
			namespace = monitor.Namespace
		}
		if configMap.Name == name && configMap.Namespace == namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
			})
		}
	}
	return requests
}

// allMonitorRequests returns reconcile requests for every CertificateMonitor.
func (r *CertificateMonitorReconciler) allMonitorRequests(ctx context.Context) []reconcile.Request {
	var monitors monitoringv1alpha1.CertificateMonitorList
	if err := r.List(ctx, &monitors); err != nil {
		log.FromContext(ctx).Error(err, "unable to list CertificateMonitors")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(monitors.Items))
	for _, monitor := range monitors.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace},
		})
	}
	return requests
}

// sendMails sends one email per recipient for a certificate that needs attention.
func (r *CertificateMonitorReconciler) sendMails(ctx context.Context, expiry time.Time, status string, secret *corev1.Secret, recipients *[]string) error {
	log := log.FromContext(ctx)
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
			emailsFailed.WithLabelValues(status).Inc()
			log.Error(err, "failed to send email", "recipient", recipient)
			return err
		}
		emailsSent.WithLabelValues(status).Inc()
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
