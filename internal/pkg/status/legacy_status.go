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

// IsReadyFn - status getter
type IsReadyFn func(*unstructured.Unstructured) (bool, error)

var legacyTypes = map[string]map[string]IsReadyFn{
	"": map[string]IsReadyFn{
		"Service":               alwaysReady,
		"Pod":                   podReady,
		"PersistentVolumeClaim": pvcReady,
	},
	"apps": map[string]IsReadyFn{
		"StatefulSet": stsReady,
		"DaemonSet":   daemonsetReady,
		"Deployment":  deploymentReady,
		"ReplicaSet":  replicasetReady,
	},
	"policy": map[string]IsReadyFn{
		"PodDisruptionBudget": pdbReady,
	},
	"batch": map[string]IsReadyFn{
		"CronJob": cronjobReady,
		"Job":     jobReady,
	},
}

// GetLegacyReadyFn - True if we handle it as a known type
func GetLegacyReadyFn(u *unstructured.Unstructured) IsReadyFn {
	gvk := u.GroupVersionKind()
	g := gvk.Group
	k := gvk.Kind
	if _, ok := legacyTypes[g]; ok {
		if fn, ok := legacyTypes[g][k]; ok {
			return fn
		}
	}
	return nil
}

func alwaysReady(u *unstructured.Unstructured) (bool, error) { return true, nil }

func compareIntFields(u *unstructured.Unstructured, field1, field2 []string, checkFuncs ...func(int, int) bool) (bool, error) {
	v1, ok, err := clientu.NestedInt(u.UnstructuredContent(), field1...)
	if err != nil {
		return true, err
	}
	if !ok {
		return false, fmt.Errorf("%v not found", field1)
	}

	v2, ok, err := clientu.NestedInt(u.UnstructuredContent(), field2...)
	if err != nil {
		return true, err
	}
	if !ok {
		return false, fmt.Errorf("%v not found", field2)
	}

	rv := true

	for _, fn := range checkFuncs {
		rv = rv && fn(v1, v2)
	}

	return rv, nil
}

func equalInt(v1, v2 int) bool { return v1 == v2 }
func geInt(v1, v2 int) bool    { return v1 >= v2 }

// Statefulset
func stsReady(u *unstructured.Unstructured) (bool, error) {
	c1, err := compareIntFields(u, []string{"status", "readyReplicas"}, []string{"spec", "replicas"}, equalInt)
	if err != nil {
		return c1, err
	}
	c2, err := compareIntFields(u, []string{"status", "currentReplicas"}, []string{"spec", "replicas"}, equalInt)
	if err != nil {
		return c2, err
	}
	return c1 && c2, nil
}

// Deployment
func deploymentReady(u *unstructured.Unstructured) (bool, error) {
	progress := true
	available := true
	conditions := clientu.GetConditions(u.UnstructuredContent())

	if len(conditions) == 0 {
		return false, fmt.Errorf("no conditions in object")
	}

	for _, c := range conditions {
		switch clientu.GetStringField(c, "type", "") {
		case "Progressing": //appsv1.DeploymentProgressing:
			// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/progress.go#L52
			status := clientu.GetStringField(c, "status", "")
			reason := clientu.GetStringField(c, "reason", "")
			if status != "True" || reason != "NewReplicaSetAvailable" {
				progress = false
			}
		case "Available": //appsv1.DeploymentAvailable:
			status := clientu.GetStringField(c, "status", "")
			if status == "False" {
				available = false
			}
		}
	}

	return progress && available, nil
}

// Replicaset
func replicasetReady(u *unstructured.Unstructured) (bool, error) {
	failure := false
	conditions := clientu.GetConditions(u.UnstructuredContent())

	for _, c := range conditions {
		switch clientu.GetStringField(c, "type", "") {
		// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/replicaset/replica_set_utils.go
		case "ReplicaFailure": //appsv1.ReplicaSetReplicaFailure
			status := clientu.GetStringField(c, "status", "")
			if status == "True" {
				failure = true
				break
			}
		}
	}

	c1, err := compareIntFields(u, []string{"status", "replicas"}, []string{"status", "readyReplicas"}, equalInt)
	if err != nil {
		return c1, err
	}
	c2, err := compareIntFields(u, []string{"status", "replicas"}, []string{"status", "availableReplicas"}, equalInt)
	if err != nil {
		return c2, err
	}

	return !failure && c1 && c2, nil
}

// Daemonset
func daemonsetReady(u *unstructured.Unstructured) (bool, error) {
	c1, err := compareIntFields(u, []string{"status", "desiredNumberScheduled"}, []string{"status", "numberAvailable"}, equalInt)
	if err != nil {
		return c1, err
	}
	c2, err := compareIntFields(u, []string{"status", "desiredNumberScheduled"}, []string{"status", "numberReady"}, equalInt)
	if err != nil {
		return c2, err
	}

	return c1 && c2, nil
}

// PVC
func pvcReady(u *unstructured.Unstructured) (bool, error) {
	val, found, err := unstructured.NestedString(u.UnstructuredContent(), "status", "phase")
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf(".status.phase not found")
	}
	return val == "Bound", nil // corev1.ClaimBound
}

// Pod
func podReady(u *unstructured.Unstructured) (bool, error) {
	conditions := clientu.GetConditions(u.UnstructuredContent())

	for _, c := range conditions {
		if clientu.GetStringField(c, "type", "") == "Ready" && (clientu.GetStringField(c, "status", "") == "True" || clientu.GetStringField(c, "reason", "") == "PodCompleted") {
			return true, nil
		}
	}
	return false, nil
}

// PodDisruptionBudget
func pdbReady(u *unstructured.Unstructured) (bool, error) {
	return compareIntFields(u, []string{"status", "currentHealthy"}, []string{"status", "desiredHealthy"}, geInt)
}

// Cronjob
func cronjobReady(u *unstructured.Unstructured) (bool, error) {
	obj := u.UnstructuredContent()
	_, ok := obj["status"]
	if !ok {
		return false, nil
	}
	return true, nil
}

// Job
func jobReady(u *unstructured.Unstructured) (bool, error) {
	complete := false
	failed := false

	conditions := clientu.GetConditions(u.UnstructuredContent())

	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/job/utils.go#L24
	for _, c := range conditions {
		status := clientu.GetStringField(c, "status", "")
		switch clientu.GetStringField(c, "type", "") {
		case "Complete":
			if status == "True" {
				complete = true
			}
		case "Failed":
			if status == "True" {
				failed = true
			}
		}
	}

	return complete || failed, nil
}
