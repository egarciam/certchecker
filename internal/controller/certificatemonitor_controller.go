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
	"path/filepath"

	// "crypto/x509"

	// "encoding/pem"
	"fmt"
	// "net/http"

	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	"egarciam.com/checkcert/internal/config"
	// email "egarciam.com/checkcert/lib/email"
)

// CertificateMonitorReconciler reconciles a CertificateMonitor object
type CertificateMonitorReconciler struct {
	client.Client
	ConfigMapName string // Name of the ConfigMap to fetch recipients
	Scheme        *runtime.Scheme
}

const (
	valid    string = "valid"
	expired  string = "expired"
	expiring string = "expiring"
)

//+kubebuilder:rbac:groups=monitoring.egarciam.com,resources=certificatemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
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
	// var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus
	if certMonitor.Spec.DiscoverInternal {
		log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "review certificates")
		certStatuses, err := r.discoverInternalCerts(ctx, certMonitor.Spec.SendMail)
		if err != nil {
			log.Error(err, "failed to discover internal certs")
		} else {
			updatedStatuses = append(updatedStatuses, certStatuses...)
		}
		// certMonitor.Status.MonitoredCertificates = updatedStatuses
		// // log.Info(fmt.Sprintf("%v", updatedStatuses))
		// if err := r.Status().Update(ctx, certMonitor); err != nil {
		// 	log.Error(err, "failed to update CertificateMonitor status")
		// 	return ctrl.Result{}, err
		// }
	}
	// } else {
	// 	log.Info("discoverInternal", fmt.Sprintf("%v", certMonitor.Spec.DiscoverInternal), "nothing would be done")
	// }
	// Review external certs
	if certMonitor.Spec.DiscoverExternal {
		certDirsList := filepath.SplitList(*config.CertDirs)
		klog.InfoS("Check certificates", "discoverExternal", certMonitor.Spec.DiscoverExternal)
		certStatuses, err := r.discoverExternalCerts(certDirsList, clientset, nodeName)
		if err != nil {
			log.Error(err, "failed to discover internal certs")
		} else {
			updatedStatuses = append(updatedStatuses, certStatuses...)
		}
	}

	certMonitor.Status.MonitoredCertificates = updatedStatuses
	// log.Info(fmt.Sprintf("%v", updatedStatuses))
	if err := r.Status().Update(ctx, certMonitor); err != nil {
		log.Error(err, "failed to update CertificateMonitor status")
		return ctrl.Result{}, err
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

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
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
