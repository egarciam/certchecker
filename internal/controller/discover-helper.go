package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
	"egarciam.com/checkcert/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Define Prometheus metrics
var (
	certExpiryGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_cert_expiry_days",
			Help: "Time in days until the certificate expires",
		},
		[]string{"status", "node", "filename"},
	)

	sslCertificateState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ssl_certificate_state",
			Help: "State of the SSL certificate (0 = expired, 1 = expiring, 2 = valid)",
		},
		[]string{"certificate"},
	)

	nodeName  string //No es necesaria como env var ya que viene en la spec del daemonset
	clientset *kubernetes.Clientset

	certDirsList []string //Rutas donde puede haber certificados en el host
	// certDirs                    = flag.String("cert-dirs", "/etc/kubernetes/pki:/etc/ssl/certs", "OS list separator separated list of directories to scan for certificates")
	// defaultWarningDays          = flag.Int("warning-expiration-days", 30, "Number of days to consider a certificate as expiring soon")
	// defaultCheckIntervalMinutes = flag.Int("check-interval-minutes", 10080, "Checking interval in minutes. Defaul 7 days (10.080 min)")
	// debug                       = flag.Bool("debug", false, "Enable debug logging")
)

// logic to search for kubernetes.io/tls secrets across namespaces.
func (r *CertificateMonitorReconciler) discoverInternalCerts(ctx context.Context, sendMail bool) ([]monitoringv1alpha1.MonitoredCertificateStatus, error) {
	var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus
	var recipients []string
	log := log.FromContext(context.Background())
	log.Info("discoverInternalCerts")
	// List all secrets of type kubernetes.io/tls
	secretList := &corev1.SecretList{}
	if err := r.List(ctx, secretList, client.MatchingFields{"type": "kubernetes.io/tls"}); err != nil {
		log.Error(err, err.Error())
		return nil, err
	}
	//Enviamos correo?
	//TODO optimizar para no hacerlo en cada ciclo
	if sendMail {
		log.Info("SendMail", fmt.Sprintf("%v", sendMail), "Envio de correo activo - habra que enviar correo con estado de los certificados")
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

		status := GetCertificateStatus(expiry)
		switch status {
		case valid:
			log.Info("Valid certificate", "name", secret.Name, "expiry date", expiry.Format(time.RFC3339))
		case expiring:
		case expired:
			log.Info("Certificate", "status", status, "name", secret.Name, "expiry date", expiry.Format(time.RFC3339), "days left", (expiry.Sub(time.Now())).Hours()/24)
			if sendMail {
				if err := r.sendMails(status, secret.Name, expiry, recipients); err != nil {
					log.Error(err, "email failed", "certificate", secret.Name, "status", status)
				} else {
					log.Info("Email sent for certificate", "name", secret.Name, "status", status)
				}
			}

		}

		certStatuses = append(certStatuses, monitoringv1alpha1.MonitoredCertificateStatus{
			Name:      fmt.Sprintf("internal-%s-%s", secret.Namespace, secret.Name),
			Status:    GetCertificateStatus(expiry),
			Expiry:    expiry.Format(time.RFC3339),
			Namespace: secret.Namespace,
		})
	}

	return certStatuses, nil
}

// func checkCerts Logicto check external certificates
func (r *CertificateMonitorReconciler) discoverExternalCerts(certDirs []string, ctx context.Context, nodeName string) ([]monitoringv1alpha1.MonitoredCertificateStatus, error) {
	var certStatuses []monitoringv1alpha1.MonitoredCertificateStatus

	//Por cada nodo del cluster levantamos un pod para hace la comprobacion

	// Fetch the list of nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		klog.Error(err, "Failed to list nodes")
		return nil, err
	}

	for _, node := range nodeList.Items {

		if isControlPlaneNode(node) {
			pod := r.createExternalNodeCheckerPod(node.Name)
			if err := r.Create(ctx, pod); err != nil {
				if !errors.IsAlreadyExists(err) {
					klog.Error(err, "Failed to create certificate checker Pod", "node", node.Name)
				}
				continue
			}
			// err := r.monitorPod(context.Background(), pod)
			// if err != nil {
			// 	klog.Error(err)
			// }
		}

	}

	return certStatuses, nil
}

// func checkCertificare Logic to check particular certificate
// TODO posiblemenete duplicada con GetCertificateStatus en discover-helper.go
func (r *CertificateMonitorReconciler) checkCertificate(path string, info os.FileInfo, err error, clientset *kubernetes.Clientset, nodeName string, certstatus []monitoringv1alpha1.MonitoredCertificateStatus) error {
	if err != nil {
		return err
	}

	if info.IsDir() || (filepath.Ext(path) != ".crt" && filepath.Ext(path) != ".pem") {
		return nil
	}

	certData, err := ioutil.ReadFile(path)
	if err != nil {
		klog.Infof("Failed to read certificate %s: %v", path, err)
		return nil
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		klog.Infof("Failed to parse PEM in %s", path)
		return nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		klog.Infof("Failed to parse certificate %s: %v", path, err)
		return nil
	}

	expiry := cert.NotAfter
	daysRemaining := time.Until(expiry).Hours() / 24

	var status string
	if daysRemaining < 0 {
		status = "EXPIRED"
	} else if daysRemaining < float64(*config.DefaultWarningDays) {
		status = "EXPIRING"
	} else {
		status = "VALID"
	}

	klog.InfoS("Certificate control:", "certificate", path, "status", status, "node", nodeName, "expiry-date", expiry, "days-remaining", daysRemaining)
	//	certExpiryGauge.WithLabelValues(status, nodeName, path).Set(daysRemaining)

	return r.annotateNode(clientset, nodeName, path, status, expiry)
}

func (r *CertificateMonitorReconciler) annotateNode(clientset *kubernetes.Clientset, nodeName, certPath, status string, expiry time.Time) error {

	node := &corev1.Node{}
	if err := r.Get(context.TODO(), client.ObjectKey{Name: nodeName}, node); err != nil {
		klog.ErrorS(err, "Failed to fetch node", "node", nodeName)
		return err
	}

	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	node.Annotations["cert-status-"+filepath.Base(certPath)] = status
	node.Annotations["cert-expiry-"+filepath.Base(certPath)] = expiry.Format(time.RFC3339)

	_, err := clientset.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		klog.Infof("Failed to update node annotation for %s: %v", certPath, err)
	}

	return err
}

// isControlPlaneNode checks if a node is a control plane node
func isControlPlaneNode(node corev1.Node) bool {
	// Control plane nodes are typically labeled with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master`
	labels := node.Labels
	_, isControlPlane := labels["node-role.kubernetes.io/control-plane"]
	_, isMaster := labels["node-role.kubernetes.io/master"]
	return isControlPlane || isMaster
}
