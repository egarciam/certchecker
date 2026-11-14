package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createExternalNodeCheckerPod creates a Pod to check certificates on a
// specific control-plane node.
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
										Name: "cert-paths-config",
									},
									Key: "CERT_DIRS",
								},
							},
						},
						{
							Name: "DEBUG",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config",
									},
									Key: "DEBUG",
								},
							},
						},
						{
							Name: "DEFAULT_WARNING_DAYS",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config",
									},
									Key: "DEFAULT_WARNING_DAYS",
								},
							},
						},
						{
							Name: "DEFAULT_CHECK_INTERVAL_MINUTES",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cert-paths-config",
									},
									Key: "DEFAULT_CHECK_INTERVAL_MINUTES",
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
							Type: &hostPathType,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}
