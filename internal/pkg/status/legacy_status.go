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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

// GetConditionsFn status getter
type GetConditionsFn func(*unstructured.Unstructured) ([]Condition, error)

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

// GetLegacyConditionsFn Return condition getter function
func GetLegacyConditionsFn(u *unstructured.Unstructured) GetConditionsFn {
	gvk := u.GroupVersionKind()
	g := gvk.Group
	k := gvk.Kind
	key := g + "/" + k
	if g == "" {
		key = k
	}
	fn, _ := legacyTypes[key]
	return fn
}

// alwaysReady Used for resources that are always ready
func alwaysReady(u *unstructured.Unstructured) ([]Condition, error) {
	return []Condition{Condition{Type: ConditionReady, Reason: "Always", Status: "True"}}, nil
}

func defaultReadyProgressConditions() (Condition, Condition) {
	return Condition{ConditionReady, "False", "", ""}, Condition{ConditionProgress, "True", "", ""}
}

// HasBeenObserved returns True if .status.observedGeneration exists and matches .metadata.generation
func HasBeenObserved(u *unstructured.Unstructured) bool {
	obj := u.UnstructuredContent()
	// ensure that the meta generation is observed
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", -1)
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	if observedGeneration != metaGeneration {
		return false
	}
	return true
}

func notObservedConditions() []Condition {
	ready, progress := defaultReadyProgressConditions()
	progress.SetReasonMessage("NotObserved", "")
	ready.SetReasonMessage("NotObserved", "Controller has not observed the latest change. Status generation does not match with metadata")
	return []Condition{ready, progress}
}

// stsConditions return standardized Conditions for Statefulset
//  Ready
//   .spec.updateStrategy.type == ondelete => True
//   .status.observedGeneration != .metadata.generation => False
//   .spec.replicas > .status.replicas => False
//   .spec.replicas > .status.readyReplicas => False
//   partition is enabled:
//     .status.updatedReplicas < .spec.Replicas - .spec.updateStrategy.rollingUpdate.partition => False
//     else True
//   .spec.replicas > .status.currentReplicas => False
//   .status.currentRevision != .status.updatedReplicas => False
//   else True
//
//  Failed => n/a
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//  Progress => True when Ready is false
//
func stsConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	ready, progress := defaultReadyProgressConditions()

	// updateStrategy==ondelete is a user managed statefulset.
	updateStrategy := clientu.GetStringField(obj, ".spec.updateStrategy.type", "")
	if updateStrategy == "ondelete" {
		ready.Status = "True"
		ready.Reason = "OnDeleteStrategy"
		return []Condition{ready}, nil
	}

	// ensure that the meta generation is observed
	if !HasBeenObserved(u) {
		return notObservedConditions(), nil
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
		progress.SetReasonMessage("LessReplicas", message)
		ready.SetReasonMessage("LessReplicas", "Waiting for requested replicas. "+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		progress.SetReasonMessage("LessReady", message)
		ready.SetReasonMessage("LessReady", "Waiting for replicas to become Ready. "+message)
		return []Condition{ready, progress}, nil
	}

	if partition != -1 {
		if updatedReplicas < (specReplicas - partition) {
			message := fmt.Sprintf("updated: %d/%d", updatedReplicas, specReplicas-partition)
			progress.SetReasonMessage("PartitionRollout", message)
			ready.SetReasonMessage("PartitionRollout", "Waiting for partition rollout to complete. "+message)
			return []Condition{ready, progress}, nil
		}
		// Partition case All ok
		ready.Status = "True"
		ready.SetReasonMessage("RolloutComplete", fmt.Sprintf("Partition rollout complete. updated: %d", updatedReplicas))
		return []Condition{ready}, nil

	}

	if specReplicas > currentReplicas {
		message := fmt.Sprintf("current: %d/%d", currentReplicas, specReplicas)
		progress.SetReasonMessage("LessCurrent", message)
		ready.SetReasonMessage("LessCurrent", "Waiting for replicas to become current. "+message)
		return []Condition{ready, progress}, nil
	}

	// Revision
	currentRevision := clientu.GetStringField(obj, ".status.currentRevision", "")
	updatedRevision := clientu.GetStringField(obj, ".status.updatedRevision", "")
	if currentRevision != updatedRevision {
		progress.SetReasonMessage("RevisionMismatch", "Pending revision update")
		ready.SetReasonMessage("RevisionMismatch", "Waiting for updated revision to match current")
		return []Condition{ready, progress}, nil
	}

	// All ok
	ready.Status = "True"
	ready.SetReasonMessage("ReplicasOK", fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", statusReplicas))
	return []Condition{ready}, nil
}

// deploymentConditions return standardized Conditions for Deployment
//  Ready
//   .status.observedGeneration != .metadata.generation => False
//   .spec.replicas > .status.updatedReplicas => False
//   .status.replicas > .status.updatedReplicas => False  "pending old replicas deletion"
//   .status.updatedReplicas > .status.availableReplicas => False
//   .spec.Replicas > .status.readyReplicas => False
//   .spec.Replicas > .status.replicas => False
//   .status.conditions[*]
//       .type==Progressing, .ready!=True OR .reason!=NewReplicaSetAvailable => False
//       .type==Progressing, .reason!=ProgressDeadlineExceeded => False
//       .type==Available, .status!=True => False
//   else True
//
//  Failed
//    .status.conditions[*] .reason!=ProgressDeadlineExceeded => True
//
//  Progress => when not Ready or not Failed
//
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//
func deploymentConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	ready, progress := defaultReadyProgressConditions()

	progressing := false
	available := false

	// ensure that the meta generation is observed
	if !HasBeenObserved(u) {
		return notObservedConditions(), nil
	}

	objc := clientu.GetObjectWithConditions(obj)

	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Progressing": //appsv1.DeploymentProgressing:
			// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/progress.go#L52
			if c.Reason == "ProgressDeadlineExceeded" {
				ready.SetReasonMessage(c.Reason, c.Message)
				progress.Status = "False"
				progress.SetReasonMessage(c.Reason, "Not Progressing")
				return []Condition{ready, progress, Condition{ConditionFailed, "True", c.Reason, c.Message}}, nil
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
		progress.SetReasonMessage("LessReplicas", message)
		ready.SetReasonMessage("LessReplicas", "Waiting for all .status.replicas to be catchup."+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > updatedReplicas {
		message := fmt.Sprintf("Updated: %d/%d", updatedReplicas, specReplicas)
		progress.SetReasonMessage("LessUpdated", message)
		ready.SetReasonMessage("LessUpdated", "Waiting for all replicas to be updated. "+message)
		return []Condition{ready, progress}, nil
	}

	if statusReplicas > updatedReplicas {
		message := fmt.Sprintf("Pending termination: %d", statusReplicas-updatedReplicas)
		progress.SetReasonMessage("ExtraPods", message)
		ready.SetReasonMessage("ExtraPods", "Waiting for old replicas to finish termination. "+message)
		return []Condition{ready, progress}, nil
	}

	if updatedReplicas > availableReplicas {
		message := fmt.Sprintf("Available: %d/%d", availableReplicas, updatedReplicas)
		progress.SetReasonMessage("LessAvailable", message)
		ready.SetReasonMessage("LessAvailable", "Waiting for all replicas to be available. "+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		progress.SetReasonMessage("LessReady", message)
		ready.SetReasonMessage("LessReady", "Waiting for all replicas to be ready. "+message)
		return []Condition{ready, progress}, nil
	}

	// check conditions
	if !progressing {
		message := "ReplicaSet not Available"
		progress.SetReasonMessage("ReplicaSetNotAvailable", message)
		ready.SetReasonMessage("ReplicaSetNotAvailable", message)
		return []Condition{ready, progress}, nil
	}
	if !available {
		message := "Deployment not Available"
		progress.SetReasonMessage("DeploymentNotAvailable", message)
		ready.SetReasonMessage("DeploymentNotAvailable", message)
		return []Condition{ready, progress}, nil
	}
	// All ok
	ready.Status = "True"
	ready.SetReasonMessage("ReplicasOK", fmt.Sprintf("Deployment is available. Replicas: %d", statusReplicas))
	return []Condition{ready}, nil
}

// replicasetConditions return standardized Conditions for Replicaset
//  Ready
//   .status.observedGeneration != .metadata.generation => False
//   .status.conditions[*]
//       .type==ReplicaFailure, .ready!=True => False
//   .spec.replicas > .status.labelledReplicas => False
//   .spec.replicas > .status.availableReplicas => False
//   .spec.replicas > .status.readyReplicas => False
//   else True
//
//  Progress => when not Ready or not Failed
//
//  Failed => n/a
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//
func replicasetConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	ready, progress := defaultReadyProgressConditions()

	// ensure that the meta generation is observed
	if !HasBeenObserved(u) {
		return notObservedConditions(), nil
	}

	// Conditions
	objc := clientu.GetObjectWithConditions(obj)
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/replicaset/replica_set_utils.go
		case "ReplicaFailure": //appsv1.ReplicaSetReplicaFailure
			if c.Status == "True" {
				message := "Replica Failure condition. Check Pods"
				ready.SetReasonMessage("ReplicaFailure", message)
				progress.SetReasonMessage("ReplicaFailure", "Replica Failure")
				return []Condition{ready, progress}, nil
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
		ready.SetReasonMessage("ZeroReplicas", message)
		progress.SetReasonMessage("ZeroReplicas", message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > labelledReplicas {
		message := fmt.Sprintf("Labelled: %d/%d", labelledReplicas, specReplicas)
		progress.SetReasonMessage("LessLabelled", message)
		ready.SetReasonMessage("LessLabelled", "Waiting for all replicas to be fully-labeled. "+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > availableReplicas {
		message := fmt.Sprintf("Available: %d/%d", availableReplicas, specReplicas)
		progress.SetReasonMessage("LessAvailable", message)
		ready.SetReasonMessage("LessAvailable", "Waiting for all replicas to be available. "+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas > readyReplicas {
		message := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		progress.SetReasonMessage("LessReady", message)
		ready.SetReasonMessage("LessReady", "Waiting for all replicas to be ready. "+message)
		return []Condition{ready, progress}, nil
	}

	if specReplicas < statusReplicas {
		message := fmt.Sprintf("replicas: %d/%d", statusReplicas, specReplicas)
		progress.SetReasonMessage("ExtraPods", message)
		ready.SetReasonMessage("ExtraPods", "Waiting for extra replicas to be removed. "+message)
		return []Condition{ready, progress}, nil
	}
	// All ok
	ready.Status = "True"
	ready.SetReasonMessage("ReplicasOK", fmt.Sprintf("ReplicaSet is available. Replicas: %d", statusReplicas))
	return []Condition{ready}, nil
}

// daemonsetConditions return standardized Conditions for DaemonSet
//  Ready
//   .status.observedGeneration != .metadata.generation => False
//   .status.desiredNumberScheduled missing => False
//   .status.desiredNumberScheduled > status.currentNumberScheduled => False
//   .status.desiredNumberScheduled > status.updatedNumberScheduled => False
//   .status.desiredNumberScheduled > status.numberAvailable => False
//   .status.desiredNumberScheduled > status.numberReady => False
//   else True
//
//  Progress => when not Ready
//
//  Failed => n/a
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//
func daemonsetConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	ready, progress := defaultReadyProgressConditions()

	// ensure that the meta generation is observed
	if !HasBeenObserved(u) {
		return notObservedConditions(), nil
	}

	// replicas
	desiredNumberScheduled := clientu.GetIntField(obj, ".status.desiredNumberScheduled", -1)
	currentNumberScheduled := clientu.GetIntField(obj, ".status.currentNumberScheduled", 0)
	updatedNumberScheduled := clientu.GetIntField(obj, ".status.updatedNumberScheduled", 0)
	numberAvailable := clientu.GetIntField(obj, ".status.numberAvailable", 0)
	numberReady := clientu.GetIntField(obj, ".status.numberReady", 0)

	if desiredNumberScheduled == -1 {
		message := "Missing .status.desiredNumberScheduled"
		progress.SetReasonMessage("NoDesiredNumber", message)
		ready.SetReasonMessage("NoDesiredNumber", message)
		return []Condition{ready, progress}, nil
	}

	if desiredNumberScheduled > currentNumberScheduled {
		message := fmt.Sprintf("Current: %d/%d", currentNumberScheduled, desiredNumberScheduled)
		progress.SetReasonMessage("LessCurrent", message)
		ready.SetReasonMessage("LessCurrent", "Waiting for desired replicas to be scheduled. "+message)
		return []Condition{ready, progress}, nil
	}

	if desiredNumberScheduled > updatedNumberScheduled {
		message := fmt.Sprintf("Updated: %d/%d", updatedNumberScheduled, desiredNumberScheduled)
		progress.SetReasonMessage("LessUpdated", message)
		ready.SetReasonMessage("LessUpdated", "Waiting for updated replicas to be scheduled. "+message)
		return []Condition{ready, progress}, nil
	}

	if desiredNumberScheduled > numberAvailable {
		message := fmt.Sprintf("Available: %d/%d", numberAvailable, desiredNumberScheduled)
		progress.SetReasonMessage("LessAvailable", message)
		ready.SetReasonMessage("LessAvailable", "Waiting for replicas to be available. "+message)
		return []Condition{ready, progress}, nil
	}

	if desiredNumberScheduled > numberReady {
		message := fmt.Sprintf("Ready: %d/%d", numberReady, desiredNumberScheduled)
		progress.SetReasonMessage("LessReady", message)
		ready.SetReasonMessage("LessReady", "Waiting for replicas to be ready. "+message)
		return []Condition{ready, progress}, nil
	}

	// All ok
	ready.Status = "True"
	ready.SetReasonMessage("ReplicasOK", fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", desiredNumberScheduled))
	return []Condition{ready}, nil
}

// pvcConditions return standardized Conditions for PVC
//  Ready
//   .status.phase != Bound => False
//   else True
//
//  Progress => when not Ready
//
//  Failed => n/a
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => n/a
//
func pvcConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	ready, progress := defaultReadyProgressConditions()

	phase := clientu.GetStringField(obj, ".status.phase", "unknown")
	if phase != "Bound" { // corev1.ClaimBound
		message := fmt.Sprintf("PVC is not Bound. phase: %s", phase)
		progress.SetReasonMessage("NotBound", message)
		ready.SetReasonMessage("NotBound", message)
		return []Condition{ready, progress}, nil
	}
	// All ok
	ready.Status = "True"
	ready.SetReasonMessage("Bound", "PVC is Bound")
	return []Condition{ready}, nil
}

// podConditions return standardized Conditions for Pod
//  Completed
//   .status.conditions[*] .type==Ready, .ready==False, .reason==PodCompleted .status.phase==Succeeded => True
//  Failed
//   .status.conditions[*] .type==Ready, .ready==False, .reason==PodCompleted .status.phase!=Succeeded => True
//  Ready
//   .status.conditions[*] .type==Ready, .ready==True, => True
//   .status.conditions[*] .type==Ready, .ready==False, .reason==PodCompleted => True
//   .status.conditions[*] .type==Ready, .ready==False, .reason!=PodCompleted => False
//
//  Progress => when not Ready, Failed, Completed
//
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => n/a
//
func podConditions(u *unstructured.Unstructured) ([]Condition, error) {
	rv := []Condition{}
	ready, progress := defaultReadyProgressConditions()
	obj := u.UnstructuredContent()
	podready := false

	phase := clientu.GetStringField(obj, ".status.phase", "unknown")
	objc := clientu.GetObjectWithConditions(obj)

	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Ready":
			podready = true
			message := "Phase: " + phase
			if c.Reason != "" {
				message += ", " + c.Reason
			}
			ready.SetReasonMessage(c.Reason, message)
			ready.Status = c.Status
			if c.Status == "False" {
				if c.Reason == "PodCompleted" {
					ready.Status = "True"
					if phase == "Succeeded" {
						rv = append(rv, Condition{ConditionCompleted, "True", "PodSucceeded", "Pod Succeeded"})
					} else {
						rv = append(rv, Condition{ConditionFailed, "True", "PodFail", fmt.Sprintf("Pod phase: %s", phase)})
					}
				} else {
					progress.SetReasonMessage(c.Reason, message)
					rv = append(rv, progress)
				}
			}
		}
	}

	if !podready {
		message := "Phase: " + phase
		ready.SetReasonMessage("PodNotReady", message)
		progress.SetReasonMessage("PodNotReady", message)
		rv = append(rv, progress)
	}
	rv = append(rv, ready)
	return rv, nil
}

// pdbConditions return standardized Conditions for Deployment
//  Ready
//   .status.desiredHealthy == 0 => False
//   .status.desiredHealthy > .status.currentHealthy => False
//   else True
//
//  Failed => n/a
//  Completed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//  Progress => not implemented
//
func pdbConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := Condition{ConditionReady, "False", "", ""}

	// replicas
	currentHealthy := clientu.GetIntField(obj, ".status.currentHealthy", 0)
	desiredHealthy := clientu.GetIntField(obj, ".status.desiredHealthy", 0)
	if desiredHealthy == 0 {
		readyCondition.Reason = "Missing or zero .status.desiredHealthy"
		return []Condition{readyCondition}, nil
	}
	if desiredHealthy > currentHealthy {
		readyCondition.Reason = fmt.Sprintf("Budget not met. healthy replicas: %d/%d", currentHealthy, desiredHealthy)
		return []Condition{readyCondition}, nil
	}

	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("Budget is met. Replicas: %d/%d", currentHealthy, desiredHealthy)
	return []Condition{readyCondition}, nil
}

// jobConditions return standardized Conditions for Job
//  Completed
//   .status.conditions[*] .type==Complete, .ready==True => True
//  Failed
//   .status.conditions[*] .type==Complete, .ready==True => True
//  Ready
//   .status.conditions[*]
//      .type==Complete, .ready==True => True
//      .type==Failed, .ready==True => True
//   .status.starttime == "" => False
//   else False
//
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//  Progress => not implemented
//
func jobConditions(u *unstructured.Unstructured) ([]Condition, error) {
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
				return []Condition{
					Condition{ConditionReady, "True", "", message},
					Condition{ConditionCompleted, "True", "", message},
				}, nil
			}
		case "Failed":
			if c.Status == "True" {
				message := fmt.Sprintf("Job Failed. failed: %d/%d", failed, completions)
				return []Condition{
					Condition{ConditionReady, "True", "", message},
					Condition{ConditionFailed, "True", "", message},
				}, nil
			}
		}
	}

	// replicas
	if starttime == "" {
		message := "Job not started"
		return []Condition{Condition{ConditionReady, "False", "", message}}, nil
	}
	message := fmt.Sprintf("Job in progress. success:%d, active: %d, failed: %d", succeeded, active, failed)
	return []Condition{Condition{ConditionReady, "True", "", message}}, nil
}

// serviceConditions return standardized Conditions for Service
//  Ready
//   .spec.type != LoadBalancer => Ready
//   .spec.clusterIP != "" Ready
//
//  Completed => n/a
//  Failed => n/a
//  Terminating => When .metadata.deletionTimestamp is set
//  Settled => not implemented
//  Progress => not implemented
//
func serviceConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()

	specType := clientu.GetStringField(obj, ".spec.type", "ClusterIP")
	specClusterIP := clientu.GetStringField(obj, ".spec.clusterIP", "")
	//statusLBIngress := clientu.GetStringField(obj, ".status.loadBalancer.ingress", "")

	message := fmt.Sprintf("Always Ready. Service type: %s", specType)
	if specType == "LoadBalancer" {
		if specClusterIP == "" {
			message = "ClusterIP not set. Service type: LoadBalancer"
			return []Condition{Condition{ConditionReady, "False", "", message}}, nil
		}
		message = fmt.Sprintf("ClusterIP: %s", specClusterIP)
	}

	return []Condition{Condition{ConditionReady, "True", "", message}}, nil
}
