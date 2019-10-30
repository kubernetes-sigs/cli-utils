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

package status

import (
	"fmt"
	
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-utils/internal/pkg/client/unstructured"
)

// GetConditionsFn defines the signature for functions to compute the
// status of a built-in resource.
type GetConditionsFn func(*unstructured.Unstructured) (*Result, error)

// legacyTypes defines the mapping from GroupKind to a function that can
// compute the status for the given resource.
var legacyTypes = map[string]GetConditionsFn{
	"Service":                    serviceConditions,
	"Pod":                        podConditions,
	"PersistentVolumeClaim":      pvcConditions,
	"apps/StatefulSet":           stsConditions,
	"apps/DaemonSet":             daemonsetConditions,
	"apps/Deployment":            deploymentConditions,
	"apps/ReplicaSet":            replicasetConditions,
	"policy/PodDisruptionBudget": pdbConditions,
	"batch/CronJob":              alwaysReady,
	"ConfigMap":                  alwaysReady,
	"batch/Job":                  jobConditions,
}

// GetLegacyConditionsFn returns a function that can compute the status for the
// given resource, or nil if the resource type is not known.
func GetLegacyConditionsFn(u *unstructured.Unstructured) GetConditionsFn {
	gvk := u.GroupVersionKind()
	g := gvk.Group
	k := gvk.Kind
	key := g + "/" + k
	if g == "" {
		key = k
	}
	return legacyTypes[key]
}

// alwaysReady Used for resources that are always ready
func alwaysReady(u *unstructured.Unstructured) (*Result, error) {
	return &Result{
		Status: CurrentStatus,
		Message: "Resource is always ready",
		Conditions: []Condition{},
	}, nil
}

// stsConditions return standardized Conditions for Statefulset
//
// StatefulSet does define the .status.conditions property, but the controller never
// actually sets any Conditions. Thus, status must be computed only based on the other
// properties under .status. We don't have any way to find out if a reconcile for a
// StatefulSet has failed.
func stsConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// updateStrategy==ondelete is a user managed statefulset.
	updateStrategy := clientu.GetStringField(obj, ".spec.updateStrategy.type", "")
	if updateStrategy == "ondelete" {
		return &Result{
			Status: CurrentStatus,
			Message: "StatefulSet is using the ondelete update strategy",
			Conditions: []Condition{},
		}, nil
	}

	// Replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	currentReplicas := clientu.GetIntField(obj, ".status.currentReplicas", 0)
	updatedReplicas := clientu.GetIntField(obj, ".status.updatedReplicas", 0)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	partition := clientu.GetIntField(obj, ".spec.updateStrategy.rollingUpdate.partition", -1)

	if specReplicas > statusReplicas {
		message := fmt.Sprintf("Replicas: %d/%d", statusReplicas, specReplicas)
		return newInProgressStatus("LessReplicas", message), nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return newInProgressStatus("LessReady", message), nil
	}

	if partition != -1 {
		if updatedReplicas < (specReplicas - partition) {
			message := fmt.Sprintf("updated: %d/%d", updatedReplicas, specReplicas-partition)
			return newInProgressStatus("PartitionRollout", message), nil
		}
		// Partition case All ok
		return &Result{
			Status: CurrentStatus,
			Message: fmt.Sprintf("Partition rollout complete. updated: %d", updatedReplicas),
			Conditions: []Condition{},
		}, nil
	}

	if specReplicas > currentReplicas {
		message := fmt.Sprintf("current: %d/%d", currentReplicas, specReplicas)
		return newInProgressStatus("LessCurrent", message), nil
	}

	// Revision
	currentRevision := clientu.GetStringField(obj, ".status.currentRevision", "")
	updatedRevision := clientu.GetStringField(obj, ".status.updatedRevision", "")
	if currentRevision != updatedRevision {
		message := "Waiting for updated revision to match current"
		return newInProgressStatus("RevisionMismatch", message), nil
	}

	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", statusReplicas),
		Conditions: []Condition{},
	}, nil
}

// deploymentConditions return standardized Conditions for Deployment.
//
// For Deployments, we look at .status.conditions as well as the other properties
// under .status. Status will be Failed if the progress deadline has been exceeded.
func deploymentConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	progressing := false
	available := false

	objc := clientu.GetObjectWithConditions(obj)

	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Progressing": //appsv1.DeploymentProgressing:
			// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/progress.go#L52
			if c.Reason == "ProgressDeadlineExceeded" {
				return &Result{
					Status:     FailedStatus,
					Message:    "Progress deadline exceeded",
					Conditions: []Condition{{ConditionFailed, "True", c.Reason, c.Message}},
				}, nil
			}
			if c.Status == "True" && c.Reason == "NewReplicaSetAvailable" {
				progressing = true
			}
		case "Available": //appsv1.DeploymentAvailable:
			if c.Status == "True" {
				available = true
			}
		}
	}

	// replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	updatedReplicas := clientu.GetIntField(obj, ".status.updatedReplicas", 0)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := clientu.GetIntField(obj, ".status.availableReplicas", 0)

	// TODO spec.replicas zero case ??

	if specReplicas > statusReplicas {
		message := fmt.Sprintf("replicas: %d/%d", statusReplicas, specReplicas)
		return newInProgressStatus("LessReplicas", message), nil
	}

	if specReplicas > updatedReplicas {
		message := fmt.Sprintf("Updated: %d/%d", updatedReplicas, specReplicas)
		return newInProgressStatus("LessUpdated", message), nil
	}

	if statusReplicas > updatedReplicas {
		message := fmt.Sprintf("Pending termination: %d", statusReplicas-updatedReplicas)
		return newInProgressStatus("ExtraPods", message), nil
	}

	if updatedReplicas > availableReplicas {
		message := fmt.Sprintf("Available: %d/%d", availableReplicas, updatedReplicas)
		return newInProgressStatus("LessAvailable", message), nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return newInProgressStatus("LessReady", message), nil
	}

	// check conditions
	if !progressing {
		message := "ReplicaSet not Available"
		return newInProgressStatus("ReplicaSetNotAvailable", message), nil
	}
	if !available {
		message := "Deployment not Available"
		return newInProgressStatus("DeploymentNotAvailable", message), nil
	}
	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("Deployment is available. Replicas: %d", statusReplicas),
		Conditions: []Condition{},
	}, nil
}

// replicasetConditions return standardized Conditions for Replicaset
func replicasetConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// Conditions
	objc := clientu.GetObjectWithConditions(obj)
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/replicaset/replica_set_utils.go
		case "ReplicaFailure": //appsv1.ReplicaSetReplicaFailure
			if c.Status == "True" {
				message := "Replica Failure condition. Check Pods"
				return newInProgressStatus("ReplicaFailure", message), nil
			}
		}
	}

	// Replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := clientu.GetIntField(obj, ".status.availableReplicas", 0)
	labelledReplicas := clientu.GetIntField(obj, ".status.labelledReplicas", 0)

	if specReplicas == 0 && labelledReplicas == 0 && availableReplicas == 0 && readyReplicas == 0 {
		message := ".spec.replica is 0"
		return newInProgressStatus("ZeroReplicas", message), nil
	}

	if specReplicas > labelledReplicas {
		message := fmt.Sprintf("Labelled: %d/%d", labelledReplicas, specReplicas)
		return newInProgressStatus("LessLabelled", message), nil
	}

	if specReplicas > availableReplicas {
		message := fmt.Sprintf("Available: %d/%d", availableReplicas, specReplicas)
		return newInProgressStatus("LessAvailable", message), nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return newInProgressStatus("LessReady", message), nil
	}

	if specReplicas < statusReplicas {
		message := fmt.Sprintf("replicas: %d/%d", statusReplicas, specReplicas)
		return newInProgressStatus("ExtraPods", message), nil
	}
	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("ReplicaSet is available. Replicas: %d", statusReplicas),
		Conditions: []Condition{},
	}, nil
}

// daemonsetConditions return standardized Conditions for DaemonSet
func daemonsetConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// replicas
	desiredNumberScheduled := clientu.GetIntField(obj, ".status.desiredNumberScheduled", -1)
	currentNumberScheduled := clientu.GetIntField(obj, ".status.currentNumberScheduled", 0)
	updatedNumberScheduled := clientu.GetIntField(obj, ".status.updatedNumberScheduled", 0)
	numberAvailable := clientu.GetIntField(obj, ".status.numberAvailable", 0)
	numberReady := clientu.GetIntField(obj, ".status.numberReady", 0)

	if desiredNumberScheduled == -1 {
		message := "Missing .status.desiredNumberScheduled"
		return newInProgressStatus("NoDesiredNumber", message), nil
	}

	if desiredNumberScheduled > currentNumberScheduled {
		message := fmt.Sprintf("Current: %d/%d", currentNumberScheduled, desiredNumberScheduled)
		return newInProgressStatus("LessCurrent", message), nil
	}

	if desiredNumberScheduled > updatedNumberScheduled {
		message := fmt.Sprintf("Updated: %d/%d", updatedNumberScheduled, desiredNumberScheduled)
		return newInProgressStatus("LessUpdated", message), nil
	}

	if desiredNumberScheduled > numberAvailable {
		message := fmt.Sprintf("Available: %d/%d", numberAvailable, desiredNumberScheduled)
		return newInProgressStatus("LessAvailable", message), nil
	}

	if desiredNumberScheduled > numberReady {
		message := fmt.Sprintf("Ready: %d/%d", numberReady, desiredNumberScheduled)
		return newInProgressStatus("LessReady", message), nil
	}

	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", desiredNumberScheduled),
		Conditions: []Condition{},
	}, nil
}

// pvcConditions return standardized Conditions for PVC
func pvcConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	phase := clientu.GetStringField(obj, ".status.phase", "unknown")
	if phase != "Bound" { // corev1.ClaimBound
		message := fmt.Sprintf("PVC is not Bound. phase: %s", phase)
		return newInProgressStatus("NotBound", message), nil
	}
	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: "PVC is Bound",
		Conditions: []Condition{},
	}, nil
}

// podConditions return standardized Conditions for Pod
func podConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()
	objc := clientu.GetObjectWithConditions(obj)
	phase := clientu.GetStringField(obj, ".status.phase", "unknown")

	if phase == "Succeeded" {
		return &Result{
			Status: CurrentStatus,
			Message: "Pod has completed successfully",
			Conditions: []Condition{},
		}, nil
	}

	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Ready":
			if c.Status == corev1.ConditionTrue {
				return &Result{
					Status: CurrentStatus,
					Message: "Pod has reached the ready state",
					Conditions: []Condition{},
				}, nil
			}
			if c.Status == corev1.ConditionFalse && c.Reason == "PodCompleted" && phase != "Succeeded" {
				message := "Pod has completed, but not successfully."
				return &Result{
					Status: FailedStatus,
					Message: message,
					Conditions: []Condition{{
						Type:    ConditionFailed,
						Status:  corev1.ConditionTrue,
						Reason:  "PodFailed",
						Message: fmt.Sprintf("Pod has completed, but not succeesfully."),
					}},
				}, nil
			}
		}
	}

	message := "Pod has not become ready"
	return newInProgressStatus("PodNotReady", message), nil
}

// pdbConditions return standardized Conditions for Deployment
func pdbConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// replicas
	currentHealthy := clientu.GetIntField(obj, ".status.currentHealthy", 0)
	desiredHealthy := clientu.GetIntField(obj, ".status.desiredHealthy", 0)
	if desiredHealthy == 0 {
		message := "Missing or zero .status.desiredHealthy"
		return newInProgressStatus("ZeroDesiredHealthy", message), nil
	}
	if desiredHealthy > currentHealthy {
		message := fmt.Sprintf("Budget not met. healthy replicas: %d/%d", currentHealthy, desiredHealthy)
		return newInProgressStatus("BudgetNotMet", message), nil
	}

	// All ok
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("Budget is met. Replicas: %d/%d", currentHealthy, desiredHealthy),
		Conditions: []Condition{},
	}, nil
}

// jobConditions return standardized Conditions for Job
//
// A job will have the InProgress status until it starts running. Then it will have the Current
// status while the job is running and after it has been completed successfully. It
// will have the Failed status if it the job has failed.
func jobConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	parallelism := clientu.GetIntField(obj, ".spec.parallelism", 1)
	completions := clientu.GetIntField(obj, ".spec.completions", parallelism)
	succeeded := clientu.GetIntField(obj, ".status.succeeded", 0)
	active := clientu.GetIntField(obj, ".status.active", 0)
	failed := clientu.GetIntField(obj, ".status.failed", 0)
	starttime := clientu.GetStringField(obj, ".status.startTime", "")

	// Conditions
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/job/utils.go#L24
	objc := clientu.GetObjectWithConditions(obj)
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Complete":
			if c.Status == "True" {
				message := fmt.Sprintf("Job Completed. succeded: %d/%d", succeeded, completions)
				return &Result{
					Status: CurrentStatus,
					Message: message,
					Conditions: []Condition{},
				}, nil
			}
		case "Failed":
			if c.Status == "True" {
				message := fmt.Sprintf("Job Failed. failed: %d/%d", failed, completions)
				return &Result{
					Status: FailedStatus,
					Message: message,
					Conditions: []Condition{{
						ConditionFailed,
						corev1.ConditionTrue,
						"JobFailed",
						fmt.Sprintf("Job Failed. failed: %d/%d", failed, completions),
					}},
				}, nil
			}
		}
	}

	// replicas
	if starttime == "" {
		message := "Job not started"
		return newInProgressStatus("JobNotStarted", message), nil
	}
	return &Result{
		Status: CurrentStatus,
		Message: fmt.Sprintf("Job in progress. success:%d, active: %d, failed: %d", succeeded, active, failed),
		Conditions: []Condition{},
	}, nil
}

// serviceConditions return standardized Conditions for Service
func serviceConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	specType := clientu.GetStringField(obj, ".spec.type", "ClusterIP")
	specClusterIP := clientu.GetStringField(obj, ".spec.clusterIP", "")

	if specType == "LoadBalancer" {
		if specClusterIP == "" {
			message := "ClusterIP not set. Service type: LoadBalancer"
			return newInProgressStatus("NoIPAssigned", message), nil
		}
	}

	return &Result{
		Status: CurrentStatus,
		Message: "Service is ready",
		Conditions: []Condition{},
	}, nil
}
