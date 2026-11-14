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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CertificateMonitorSpec defines the desired state of CertificateMonitor

type CertificateMonitorSpec struct {
	// DiscoverInternal enables discovery of kubernetes.io/tls Secrets across
	// the cluster.
	// +kubebuilder:default=false
	DiscoverInternal bool `json:"discoverInternal,omitempty"`
	// SendMail enables email notifications for expiring and expired
	// certificates.
	// +kubebuilder:default=false
	SendMail bool `json:"sendMail,omitempty"`
	// CheckInterval is the number of seconds between certificate checks.
	// +kubebuilder:default=86400
	// +kubebuilder:validation:Minimum=0
	CheckInterval int `json:"checkInterval,omitempty"`
	// EmailCooldown is the number of seconds to wait before re-notifying about
	// the same certificate.
	// +kubebuilder:default=86400
	// +kubebuilder:validation:Minimum=0
	EmailCooldown int `json:"emailCooldown,omitempty"`
	// ExpiryThresholdDays is the number of days before expiry at which a
	// certificate is reported as expiring.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=0
	ExpiryThresholdDays int `json:"expiryThresholdDays,omitempty"`
	// EmailRecipientsConfigMap is the name of the ConfigMap containing the
	// JSON list of email recipients under the "emails" key.
	// +kubebuilder:default=email-recipients-config
	EmailRecipientsConfigMap string `json:"emailRecipientsConfigMap,omitempty"`
	// EmailRecipientsNamespace is the namespace of the recipients ConfigMap.
	// It defaults to the namespace of the CertificateMonitor object.
	EmailRecipientsNamespace string `json:"emailRecipientsNamespace,omitempty"`
}

// MonitoredCertificateStatus represents the status of a monitored certificate.
type MonitoredCertificateStatus struct {
	Name            string `json:"name"`
	Status          string `json:"status"`           // "valid", "expiring", "expired"
	Expiry          string `json:"expiry,omitempty"` //Fecha de validez
	Namespace       string `json:"namespace"`
	EmailSent       bool   `json:"emailSent"`       // Add a flag
	LastEmailSentAt string `json:"lastEmailSentAt"` //Fecha de envio de ultimo correo
}

// CertificateMonitorSummary contains aggregate certificate counts for the
// last reconciliation.
type CertificateMonitorSummary struct {
	Total    int `json:"total"`
	Valid    int `json:"valid"`
	Expiring int `json:"expiring"`
	Expired  int `json:"expired"`
	Errors   int `json:"errors,omitempty"`
}

// CertificateMonitorStatus defines the observed state of CertificateMonitor
type CertificateMonitorStatus struct {
	// ObservedGeneration is the last generation reconciled by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastChecked is the time of the last successful certificate discovery.
	LastChecked string `json:"lastChecked,omitempty"`
	// Summary contains aggregate counts from the last discovery.
	Summary CertificateMonitorSummary `json:"summary,omitempty"`
	// Conditions describe the current state of the monitor.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// MonitoredCertificates contains the certificates that are expiring or
	// expired. Valid certificates are counted in the summary.
	MonitoredCertificates []MonitoredCertificateStatus `json:"monitoredCertificates,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
//+kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.summary.total`
//+kubebuilder:printcolumn:name="Expiring",type=integer,JSONPath=`.status.summary.expiring`
//+kubebuilder:printcolumn:name="Expired",type=integer,JSONPath=`.status.summary.expired`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CertificateMonitor is the Schema for the certificatemonitors API
// +kubebuilder:resource:shortName=cmt;cmts
type CertificateMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CertificateMonitorSpec   `json:"spec,omitempty"`
	Status CertificateMonitorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CertificateMonitorList contains a list of CertificateMonitor
type CertificateMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CertificateMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CertificateMonitor{}, &CertificateMonitorList{})
}
