/*
Copyright 2018 The Kubernetes Authors.

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

package vm

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
)

// upstreamMachineClusterIDLabel is the label that a machine must have to identify the cluster to which it belongs
const upstreamMachineClusterIDLabel = "sigs.k8s.io/cluster-api-cluster"

// existingInstanceStates returns the list of states an EC2 instance can be in
// while being considered "existing", i.e. mostly anything but "Terminated".
// func existingInstanceStates() []*string {
// 	return []*string{
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.VmPhaseUnset),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Pending),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Scheduling),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Scheduled),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Running),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Succeeded),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Failed),
// 		kubevirtapiv1.VirtualMachineInstancePhaseToString(kubevirtapiv1.Unknown),
// 	}
// }

// setKubevirtMachineProviderCondition sets the condition for the machine and
// returns the new slice of conditions.
// If the machine does not already have a condition with the specified type,
// a condition will be added to the slice
// If the machine does already have a condition with the specified type,
// the condition will be updated if either of the following are true.
func setKubevirtMachineProviderCondition(condition kubevirtapiv1.VirtualMachineCondition, conditions []kubevirtapiv1.VirtualMachineCondition) []kubevirtapiv1.VirtualMachineCondition {
	now := metav1.Now()

	if existingCondition := findProviderCondition(conditions, condition.Type); existingCondition == nil {
		condition.LastProbeTime = now
		condition.LastTransitionTime = now
		conditions = append(conditions, condition)
	} else {
		updateExistingCondition(&condition, existingCondition)
	}

	return conditions
}

func findProviderCondition(conditions []kubevirtapiv1.VirtualMachineCondition, conditionType kubevirtapiv1.VirtualMachineConditionType) *kubevirtapiv1.VirtualMachineCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func updateExistingCondition(newCondition, existingCondition *kubevirtapiv1.VirtualMachineCondition) {
	if !shouldUpdateCondition(newCondition, existingCondition) {
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.LastTransitionTime = metav1.Now()
	}
	existingCondition.Status = newCondition.Status
	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
	existingCondition.LastProbeTime = newCondition.LastProbeTime
}

func shouldUpdateCondition(newCondition, existingCondition *kubevirtapiv1.VirtualMachineCondition) bool {
	return newCondition.Reason != existingCondition.Reason || newCondition.Message != existingCondition.Message
}

// The network info is saved in the vmi
// extractNodeAddresses maps the instance information from Vmi to an array of NodeAddresses
func extractNodeAddresses(vmi *kubevirtapiv1.VirtualMachineInstance) ([]corev1.NodeAddress, error) {
	// Not clear if the order matters here, but we might as well indicate a sensible preference order

	if vmi == nil {
		return nil, fmt.Errorf("nil vmi passed to extractNodeAddresses")
	}

	addresses := []corev1.NodeAddress{}
	interfaces := vmi.Status.Interfaces
	for _, i := range interfaces {
		if i.IP != "" {
			addresses = append(addresses, corev1.NodeAddress{Type: corev1.NodeInternalIP, Address: i.IP})
		}
	}

	return addresses, nil
}

// TODO There is only one kind of VirtualMachineConditionType: VirtualMachineFailure
//      How should report on success?
//      Is Failure/false is good enough or need to add type to client-go?
func conditionSuccess() kubevirtapiv1.VirtualMachineCondition {
	return kubevirtapiv1.VirtualMachineCondition{
		Type:    kubevirtapiv1.VirtualMachineFailure,
		Status:  corev1.ConditionFalse,
		Reason:  "MachineCreationSucceeded",
		Message: "Machine successfully created",
	}
}

func conditionFailed() kubevirtapiv1.VirtualMachineCondition {
	return kubevirtapiv1.VirtualMachineCondition{
		Type:   kubevirtapiv1.VirtualMachineFailure,
		Status: corev1.ConditionTrue,
		Reason: "MachineCreationFailed",
	}
}

// validateMachine check the label that a machine must have to identify the cluster to which it belongs is present.
func validateMachine(machine machinev1.Machine) error {
	// TODO: insert a validation on machine labels
	if machine.Labels[machinev1.MachineClusterIDLabel] == "" {
		return machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1.MachineClusterIDLabel)
	}

	return nil
}

// getClusterID get cluster ID by machine.openshift.io/cluster-api-cluster label
func getClusterID(machine *machinev1.Machine) (string, bool) {
	clusterID, ok := machine.Labels[machinev1.MachineClusterIDLabel]
	// NOTE: This block can be removed after the label renaming transition to machine.openshift.io
	if !ok {
		clusterID, ok = machine.Labels[upstreamMachineClusterIDLabel]
	}
	return clusterID, ok
}
