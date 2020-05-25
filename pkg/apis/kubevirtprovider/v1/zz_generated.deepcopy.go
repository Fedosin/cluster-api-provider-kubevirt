// +build !ignore_autogenerated

/*
Copyright 2019 The Kubernetes Authors.

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

package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubevirtMachineProviderSpec) DeepCopyInto(out *KubevirtMachineProviderSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubevirtMachineProviderSpec.
func (in *KubevirtMachineProviderSpec) DeepCopy() *KubevirtMachineProviderSpec {
	if in == nil {
		return nil
	}
	out := new(KubevirtMachineProviderSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubevirtMachineProviderStatus) DeepCopyInto(out *KubevirtMachineProviderStatus) {
	*out = *in
	in.VirtualMachineStatus.DeepCopyInto(&out.VirtualMachineStatus)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubevirtMachineProviderStatus.
func (in *KubevirtMachineProviderStatus) DeepCopy() *KubevirtMachineProviderStatus {
	if in == nil {
		return nil
	}
	out := new(KubevirtMachineProviderStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KubevirtMachineProviderStatus) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
