package controller

import (
	"context"
	"encoding/json"

	"egarciam.com/checkcert/internal/email"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

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
		if err := email.SendMail(subject, body, recipient); err != nil {
			log.Error(err, "failed to send email", "recipient", recipient)
			return err
		} else {
			log.Info("Email sent successfully", "recipient", recipient, "certificate", name)
		}
	}
	return nil
}
