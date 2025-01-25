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
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
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
	klog.Info("EN RECONCILIATION LOOP")
	klog.Info("Prepare to repel boarders for fist time")

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

// logic to search for kubernetes.io/tls secrets across namespaces.
func (r *CertificateMonitorReconciler) discoverInternalCerts(ctx context.Context, certmonitor *monitoringv1alpha1.CertificateMonitor) ([]monitoringv1alpha1.MonitoredCertificateStatus, error) {
	var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus
	var recipients []string
	var certStatus string
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
	//si no hay secrets coincidentes salimos
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
			return nil, err
		}
		log.Info("Recipients fetched successfully", "recipients", recipients)
	}

	//Secret Loop
	for _, secret := range secretList.Items {
		expiry, err := r.getInternalCertExpiry(ctx, secret.Namespace, secret.Name)
		if err != nil {
			log.Error(err, err.Error())
			continue // Handle error or log it
		}
		shouldSendMail := true // a priori lo enviamos (caso de certificados nuevos o que sean antiguos y se haya excedido el coolDown)
		totals.total++

		status := getCertificateStatus(expiry)
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
		//mostramos log
		log.Info(certStatus, "name", secret.Namespace+"."+secret.Name, "expiry date", expiry.Format(time.RFC3339))

		//Si llegamos aqui son certificados chungos
		// Check if email was already sent
		// emailAlreadySent := false

		var tmpStatus monitoringv1alpha1.MonitoredCertificateStatus
		for _, monitoredCert := range certmonitor.Status.MonitoredCertificates {
			if monitoredCert.Name == fmt.Sprintf("internal-%s.%s", secret.Namespace, secret.Name) && monitoredCert.EmailSent {
				tmpStatus = monitoredCert
				if shouldSendMail = emailShouldBeSent(monitoredCert, time.Duration(certmonitor.Spec.EmailCoolDown)); !shouldSendMail {
					break
				}
			}
		}
		var certstatus monitoringv1alpha1.MonitoredCertificateStatus
		//Actualizamos el status
		if shouldSendMail {
			//Hay que actaulizar las fechas
			certstat := monitoringv1alpha1.MonitoredCertificateStatus{
				Name:            fmt.Sprintf("internal-%s.%s", secret.Namespace, secret.Name),
				Status:          status,
				Expiry:          expiry.Format(time.RFC3339),
				Namespace:       secret.Namespace,
				EmailSent:       certmonitor.Spec.SendMail && shouldSendMail,
				LastEmailSentAt: getLastEmailSentAtValue(certmonitor.Spec.SendMail, certmonitor.Spec.SendMail && shouldSendMail), /*time.Now().Format(time.RFC3339) || nil*/
			}
			certstatus = certstat
		} else {
			certstatus = tmpStatus
		}

		//Send mail?
		if certmonitor.Spec.SendMail && shouldSendMail {
			err := r.sendMails(expiry, status, &secret, &recipients)
			if err != nil {
				log.Error(err, "Error al enviar el correo para el CERTIFICADO")
			}
		}

		//Update status
		certStatuses = append(certStatuses, certstatus)

		// //Send mail?
		// if certmonitor.Spec.SendMail && shouldSendMail /*&& !emailAlreadySent*/ {
		// 	if err := r.sendMails(expiry, status, &secret, &recipients); err == nil {
		// 		//status with mail sent merked
		// 		certStatuses = append(certStatuses, monitoringv1alpha1.MonitoredCertificateStatus{
		// 			Name:            fmt.Sprintf("internal-%s.%s", secret.Namespace, secret.Name),
		// 			Status:          status,
		// 			Expiry:          expiry.Format(time.RFC3339),
		// 			Namespace:       secret.Namespace,
		// 			EmailSent:       true,
		// 			LastEmailSentAt: time.Now().Format(time.RFC3339),
		// 		})

		// 	} else {
		// 		//Status without mail sent mark
		// 		// recuperamos la info del estado del certmonitor
		// 		certStatuses = append(certStatuses, monitoringv1alpha1.MonitoredCertificateStatus{
		// 			Name:            fmt.Sprintf("internal-%s.%s", secret.Namespace, secret.Name),
		// 			Status:          status,
		// 			Expiry:          expiry.Format(time.RFC3339),
		// 			Namespace:       secret.Namespace,
		// 			EmailSent:       tmpStatus.EmailSent,
		// 			LastEmailSentAt: tmpStatus.LastEmailSentAt,
		// 		})
		// 	}
	}
	//}
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

// funcion para enviar los correos a la lista de recipients
func (r *CertificateMonitorReconciler) sendMails(expiry time.Time, status string, secret *corev1.Secret, recipients *[]string) error {
	log := log.FromContext(context.Background())
	var subject, body string
	totals := struct {
		expiring int
		expired  int
		total    int
	}{0, 0, 0}

	//Prepare subect and body of mails
	switch status {
	case expiring:
		subject = fmt.Sprintf("Certificate Expiring in %v days: %s.%s", math.Round(time.Until(expiry).Hours()/24), secret.Namespace, secret.Name)
		body = fmt.Sprintf("The certificate %s.%s is expiring on %s. %v days left", secret.Namespace, secret.Name, expiry.Format(time.RFC3339), math.Round(time.Until(expiry).Hours()/24))
	case expired:
		subject = fmt.Sprintf("Certificate Expired: %s.%s", secret.Namespace, secret.Name)
		body = fmt.Sprintf("The certificate %s.%s has expired on %s.", secret.Namespace, secret.Name, expiry.Format(time.RFC3339))
	}

	//Send mails
	for _, recipient := range *recipients {
		switch status {
		case expiring:
			totals.expiring++
		case expired:
			totals.expired++
		}
		totals.total++
		// Send the email
		if err := sendMail(subject, body, recipient); err != nil {
			log.Error(err, "failed to send email", "recipient", recipient)
			return err
		}
		log.Info("Email sent successfully", "recipient", recipient, "certificate", secret.Name, "namespace", secret.Namespace)
	}
	log.Info("Sent email figures", "certificate", secret.Name, "namespace",
		secret.Namespace, "expiring", totals.expiring, "expired", totals.expired, "total", totals.total)
	return nil
}
