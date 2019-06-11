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
	"context"
	"fmt"
	"io"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	clientu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
)

// Condition types
const (
	// Level Conditions

	// ConditionReady Indicates the object is resource for use
	ConditionReady ConditionType = "Ready"
	// ConditionSettled Indicates the controller is done reconciling the spec
	// This is not implemented yet
	ConditionSettled ConditionType = "Settled"

	// Terminal condition

	// ConditionFailed The resource is in failed condition and the controller will not process it further
	ConditionFailed ConditionType = "Failed"
	// ConditionCompleted The resource is done doing what it intends. Example Job, Pods can have completed state.
	ConditionCompleted ConditionType = "Completed"
	// Terminating Indicates the resource is being deleted.
	ConditionTermination ConditionType = "Terminating"

	// Progress condition

	// ConditionProgress Indicates the controller is still working to satisfy the intent in the resource spec.
	ConditionProgress ConditionType = "Progress"
)

// Status returns the status for rollouts
type Status struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client
	// Out stores the output
	Out io.Writer
	// Resources is a list of resource configurations
	Resources clik8s.ResourceConfigs
	// Commit is a git commit object
	Commit *object.Commit
}

// ConditionType condition types
type ConditionType string

// Condition condition object computed by status package
type Condition struct {
	// Type condition type
	Type ConditionType
	// Status String that describes the condition status
	Status string // metav1.ConditionStatus
	// Reason one work CamleCase reason
	Reason string
	// Message Human readable reason string
	Message string
}

// ResourceStatus resource status
type ResourceStatus struct {
	// Resource unstructured object whose resource is being described
	Resource *unstructured.Unstructured // Deletion in progress
	// Conditions list of extracted conditions from Resource
	Conditions []Condition
	// Errror Any error encountered extracting status for this Resource
	Error error
}

// Result contains the Status Result
type Result struct {
	// Resources list of resource status
	Resources []ResourceStatus
}

// GetCondition Returns the condition matching the type
func GetCondition(cs []Condition, ct ConditionType) *Condition {
	for i := range cs {
		if cs[i].Type == ct {
			return &cs[i]
		}
	}
	return nil
}

// Do works on the list of resources and computes status for the resources.
func (a *Status) Do() Result {
	var rs = []ResourceStatus{}

	ctx := context.Background()
	for _, u := range a.Resources {
		err := a.DynamicClient.Get(ctx,
			types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, u)
		if err != nil {
			if errors.IsNotFound(err) {
			}
			rs = append(rs, ResourceStatus{Resource: u, Error: err})
			continue
		}

		// Ready indicator is a simple ANDing of all the individual resource readiness
		conditions, err := GetConditions(u)
		if err != nil {
			rs = append(rs, ResourceStatus{Resource: u, Error: err})
			continue
		}
		rs = append(rs, ResourceStatus{Resource: u, Conditions: conditions, Error: nil})
	}

	a.OutputResult(rs)
	return Result{Resources: rs}
}

// OutputResult print to output writer
func (a *Status) OutputResult(resources []ResourceStatus) {
	for i := range resources {
		u := resources[i].Resource
		fmt.Fprintf(a.Out, "%s/%s   ", u.GetKind(), u.GetName())
		outputConditions(a.Out, resources[i].Conditions)
		outputError(a.Out, resources[i].Error)
		fmt.Fprintf(a.Out, "\n")
	}
}

// GetConditions Return a list of standardizes conditions for the given unstructured object
func GetConditions(u *unstructured.Unstructured) ([]Condition, error) {
	var conditions []Condition
	var err error

	fn := GetLegacyConditionsFn(u)
	if fn == nil {
		fn = GetGenericConditionsFn(u)
	}

	if fn != nil {
		conditions, err = fn(u)
	}

	conditions = addTerminationCondition(u, conditions)

	return conditions, err
}

// SetReasonMessage set
func (s *Condition) SetReasonMessage(reason, message string) {
	s.Reason = reason
	s.Message = message
}

// addTerminationCondition injects termination condition if applicable
func addTerminationCondition(u *unstructured.Unstructured, conditions []Condition) []Condition {

	deletionTimestamp := clientu.GetStringField(u.UnstructuredContent(), ".metadata.deletionTimestamp", "")
	finalizers := u.GetFinalizers()
	if deletionTimestamp != "" {
		reason := "Terminating"
		if len(finalizers) != 0 {
			reason += fmt.Sprintf(" finalizers: %s", finalizers)
		}
		conditions = append(conditions, Condition{ConditionTermination, "True", reason, ""})
	}
	return conditions
}

func outputConditions(out io.Writer, sc []Condition) {
	ready := GetCondition(sc, ConditionReady)
	progress := GetCondition(sc, ConditionProgress)
	if ready != nil {
		if ready.Status == "True" {
			fmt.Fprintf(out, "Ready")
		} else {
			fmt.Fprintf(out, "Pending")
		}
	}
	terminating := GetCondition(sc, ConditionTermination)
	if terminating != nil {
		if terminating.Status == "True" {
			fmt.Fprintf(out, " %s", terminating.Reason)
		}
	}
	if progress != nil && progress.Status == "True" {
		fmt.Fprintf(out, " %s", progress.Message)
	}
}

func outputError(out io.Writer, err error) {
	if err == nil {
		return
	}
	if errors.IsNotFound(err) {
		fmt.Fprintf(out, " Not Found")
	} else {
		fmt.Fprintf(out, " ERR: %s", err)
	}
}

// StableOrTerminal returns True if all of the resources are stable or terminal
func StableOrTerminal(resources []ResourceStatus) bool {
	ok := true
	for i := range resources {
		ready := GetCondition(resources[i].Conditions, ConditionReady)
		if ready != nil {
			if ready.Status != "True" {
				ok = false
				break
			}
		} else {
			ok = false
			break
		}
	}
	return ok
}
