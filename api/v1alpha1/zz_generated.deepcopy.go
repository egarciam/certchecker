//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CertificateMonitor) DeepCopyInto(out *CertificateMonitor) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CertificateMonitor.
func (in *CertificateMonitor) DeepCopy() *CertificateMonitor {
	if in == nil {
		return nil
	}
	out := new(CertificateMonitor)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CertificateMonitor) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CertificateMonitorList) DeepCopyInto(out *CertificateMonitorList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CertificateMonitor, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CertificateMonitorList.
func (in *CertificateMonitorList) DeepCopy() *CertificateMonitorList {
	if in == nil {
		return nil
	}
	out := new(CertificateMonitorList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CertificateMonitorList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CertificateMonitorSpec) DeepCopyInto(out *CertificateMonitorSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CertificateMonitorSpec.
func (in *CertificateMonitorSpec) DeepCopy() *CertificateMonitorSpec {
	if in == nil {
		return nil
	}
	out := new(CertificateMonitorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CertificateMonitorStatus) DeepCopyInto(out *CertificateMonitorStatus) {
	*out = *in
	if in.MonitoredCertificates != nil {
		in, out := &in.MonitoredCertificates, &out.MonitoredCertificates
		*out = make([]MonitoredCertificateStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CertificateMonitorStatus.
func (in *CertificateMonitorStatus) DeepCopy() *CertificateMonitorStatus {
	if in == nil {
		return nil
	}
	out := new(CertificateMonitorStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoredCertificateStatus) DeepCopyInto(out *MonitoredCertificateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoredCertificateStatus.
func (in *MonitoredCertificateStatus) DeepCopy() *MonitoredCertificateStatus {
	if in == nil {
		return nil
	}
	out := new(MonitoredCertificateStatus)
	in.DeepCopyInto(out)
	return out
}
