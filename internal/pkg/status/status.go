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

// The status package computes the status for Kubernetes resources. Status in this context
// mean a single value that can be used by tools and/or developers to determine if resources
// has been fully reconciled, has reached a failure state or is still in progress. For custom
// resources this is based on a convention that the creators of CRDs must follow while for the
// built-in resources, the status is computed from the fields available in the .status section
// of the resource manifest.
// The package will also return a set of standard conditions for each resource. This is
// used to amend the conditions already available in some of the standard Kubernetes resources.
package status

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/internal/pkg/client"
	"sigs.k8s.io/cli-utils/internal/pkg/clik8s"
)

const (
	// The set of standard conditions defined in this package. These follow the "abnormality-true"
	// convention where conditions should have a true value for abnormal/error situations and the absence
	// of a condition should be interpreted as a false value, i.e. everything is normal.
	ConditionFailed ConditionType = "Failed"
	ConditionInProgress ConditionType = "InProgress"

	// The set of status conditions which can be assigned to resources.
	InProgressStatus Status = "InProgress"
	FailedStatus Status = "Failed"
	CurrentStatus Status = "Current"
	TerminatingStatus Status = "Terminating"
	UnknownStatus Status = "Unknown"
)

// Defines the set of condition types allowed inside a Condition struct.
type ConditionType string

// Defines the set of statuses a resource can have.
type Status string

// String returns the status as a string.
func (s Status) String() string {
	return string(s)
}

// Resolver can compute the Status and Conditions for resources.
type Resolver struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client
}

// Condition defines the general format for conditions on Kubernetes resources.
// In practice, each kubernetes resource defines their own format for conditions, but
// most (maybe all) follows this structure.
type Condition struct {
	// Type condition type
	Type ConditionType
	// Status String that describes the condition status
	Status corev1.ConditionStatus
	// Reason one work CamelCase reason
	Reason string
	// Message Human readable reason string
	Message string
}

// Result defines the result of computing the status for a single resource. It will
// contain the status for the resource, a message fields with more details about the
// status, and a list of the standard conditions that could be amended to the resource.
type Result struct {
	//Status
	Status Status
	// Message
	Message string
	// Conditions list of extracted conditions from Resource
	Conditions []Condition
}

// ResourceResult wraps the Result together with the Resource from which the
// status was computed as well as the error if one was encountered while computing
// the status.
type ResourceResult struct {
	Result *Result

	Resource *unstructured.Unstructured

	Error error
}

// FetchResourcesAndGetStatus computes the status for all the resources provided and returns a slice of
// ResourceResult. The resources does NOT need to be complete as the up-to-date state for all the resources
// will be fetched from the API server before computing the status.
func (r *Resolver) FetchResourcesAndGetStatus(resources clik8s.ResourceConfigs) []ResourceResult {
	var rs []ResourceResult

	ctx := context.Background()
	for _, u := range resources {
		err := r.DynamicClient.Get(ctx,
			types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, u)
		if err != nil {
			rs = append(rs, ResourceResult{Resource: u, Error: err})
			continue
		}

		res, err := GetStatus(u)
		if err != nil {
			rs = append(rs, ResourceResult{Resource: u, Error: err})
			continue
		}
		rs = append(rs, ResourceResult{Resource: u, Result: res})
	}
	return rs
}

// GetStatus computes the status for the given resource. The state of the resource will NOT be
// fetched from the API server, so the resource passed in must already have the status field
// populated.
func GetStatus(u *unstructured.Unstructured) (*Result, error) {
	res, err := checkGenericProperties(u)
	if err != nil || res != nil {
		return res, err
	}

	fn := GetLegacyConditionsFn(u)
	if fn != nil {
		return fn(u)
	}

	// The resource is not one of the built-in types with specific
	// rules and we were unable to make a decision based on the
	// generic rules. At this point we don't really know the status.
	return &Result{
		Status: UnknownStatus,
		Message: fmt.Sprintf("Unknown type %s", u.GetKind()),
		Conditions: []Condition{},
	}, err
}

// newInProgressCondition creates an inProgress condition with the given
// reason and message.
func newInProgressCondition(reason, message string) Condition {
	return Condition{
		Type: ConditionInProgress,
		Status: corev1.ConditionTrue,
		Reason: reason,
		Message: message,
	}
}

// newInProgressStatus creates a status Result with the InProgress status
// and an InProgress condition.
func newInProgressStatus(reason, message string) *Result {
	return &Result{
		Status: InProgressStatus,
		Message: message,
		Conditions: []Condition{newInProgressCondition(reason, message)},
	}
}
