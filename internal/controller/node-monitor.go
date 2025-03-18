package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// createCertificateCheckerPod creates a Pod to check certificates on a specific node
func (r *CertificateMonitorReconciler) createExternalNodeCheckerPod(nodeName string) *corev1.Pod {
	hostPathType := corev1.HostPathDirectory
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("certificate-checker-%s", nodeName),
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "node-cert-checker",
			NodeName:           nodeName,
			Containers: []corev1.Container{
				{
					Name:  "cert-checker",
					Image: "localhost:5000/node-cert-checker",
					// Command: []string{"/bin/sh", "-c", "/check_certs.sh"},
					Env: []corev1.EnvVar{
						{
							Name:  "NODE_NAME",
							Value: nodeName,
						},
						{
							Name: "CERT_DIRS",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config", // Name of the ConfigMap
									},
									Key: "CERT_DIRS", // Key in the ConfigMap
								},
							},
						},
						{
							Name: "DEBUG",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config", // Name of the ConfigMap
									},
									Key: "DEBUG", // Key in the ConfigMap
								},
							},
						},
						{
							Name: "DEFAULT_WARNING_DAYS",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config", // Name of the ConfigMap
									},
									Key: "DEFAULT_WARNING_DAYS", // Key in the ConfigMap
								},
							},
						},
						{
							Name: "DEFAULT_CHECK_INTERVAL_MINUTES",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config", // Name of the ConfigMap
									},
									Key: "DEFAULT_CHECK_INTERVAL_MINUTES", // Key in the ConfigMap
								},
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host-certs",
							MountPath: "/etc/kubernetes/pki",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "host-certs",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/etc/kubernetes/pki",
							Type: &hostPathType, //pointer.HostPathType(corev1.HostPathDirectory),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

// monitorPod monitors the status of the certificate checker Pod
func (r *CertificateMonitorReconciler) monitorPod(ctx context.Context, pod *corev1.Pod) error {
	log := log.FromContext(ctx)

	// Wait for the Pod to complete
	for {
		time.Sleep(5 * time.Second)
		if err := r.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
			return err
		}

		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			log.Info("Certificate checker Pod succeeded", "pod", pod.Name)
			return nil
		case corev1.PodFailed:
			log.Info("Certificate checker Pod failed", "pod", pod.Name)
			return fmt.Errorf("Pod %s failed", pod.Name)
		default:
			log.Info("Certificate checker Pod is still running", "pod", pod.Name)
		}
	}
}
