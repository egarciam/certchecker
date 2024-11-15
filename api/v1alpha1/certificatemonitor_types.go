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

// CertificateSpec defines the desired certificates to monitor.
type CertificateSpec struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "internal" or "external"
	SecretName string `json:"secretName,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	URL        string `json:"url,omitempty"`
}

// CertificateMonitorSpec defines the desired state of CertificateMonitor
type CertificateMonitorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Certificates     []CertificateSpec `json:"certificates"`
	DiscoverInternal bool `json:"discoverInternal,omitempty"`
}

// MonitoredCertificateStatus represents the status of a monitored certificate.
type MonitoredCertificateStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "valid", "expiring", "expired"
	Expiry string `json:"expiry,omitempty"`
}

// CertificateMonitorStatus defines the observed state of CertificateMonitor
type CertificateMonitorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	MonitoredCertificates []MonitoredCertificateStatus `json:"monitoredCertificates"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CertificateMonitor is the Schema for the certificatemonitors API
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
