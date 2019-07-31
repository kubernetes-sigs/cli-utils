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

package apply

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Apply configures how to Apply a collection of Resource Config.  The Apply config file will be picked up
// by Apply automatically, and should live in the directory that is Applied.
type Apply struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Config defines
	Config Config `json:"config,omitempty"`

	// Set defines metadata to set on the objects that are Applied
	SetMeta SetMeta `json:"setMeta,omitempty"`

	// Wait will cause further Targets to block on the completion of this Target.
	Wait bool `json:"wait,omitempty"`

	// SuccessOnAll for is a list of conditions to watch for to consider the roll out for this target to be successful.
	// Apply will block until *all* of the SuccessOnAll conditions are met, or the target is considered failed.
	SuccessOnAll []SuccessOnAll `json:"successOnAll,omitempty"`

	// FailOnAny is a list of conditions to watch for to consider the roll out for this target to be failed.
	// Apply will block until *any* of the FailOnAny conditions are met, or the target is considered successful.
	FailOnAny []FailOnAny `json:"failOnAny,omitempty"`
}

// SetMeta defines metadata to set when Applying objects.
type SetMeta struct {
	// CommitLabels will add set experimental.cli.sigs.k8s.io/commit=<commit hash> label when it Apply's Resources.
	// This can be used by GitOps solutions for provenance.
	CommitLabels bool
}

// SuccessOnAll defines conditions for the list of *applied* Resources for the roll out to be considered successful
// from Apply's perspective.
type SuccessOnAll struct {
	// MinWait is the minimum time to wait before declaring a roll out successful.  This can be used to ensure
	// the Target remains healthy and non-failing.
	MinWait time.Duration `json:"timeout,omitempty"`

	// Only apply these conditions to Resources of this type from the list of applied Resources.
	// Filter all Resources if unspecified.
	Resource metav1.GroupVersionKind `json:"resource,omitempty"`

	// Selector filters the conditions to a specific set of Resources.  Filter all Resources if unspecified.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Status is the Status for Resources that were provided to Apply.  All matching Resources must have this
	// Status.
	Status Status `json:"status,omitempty"`

	// Conditions are Conditions for Resources that were provided to Apply.  All matching Resources must
	// have these Conditions.
	Conditions map[string]string `json:"conditions,omitempty"`

	// Logs defines a list of regex to match Pod logs.  Only looks at Pods matching Selector.
	// Will follow logs until this regex is matched.  Requires Selector to be specified, and only
	// matches Pods matching the selector.
	Logs []string `json:"logs,omitempty"`

	// ApplicationMetrics are ApplicationMetrics from Prometheus or Stackdriver to watch for.
	ApplicationMetrics ApplicationMetrics `json:"applicationMetrics,omitempty"`
}

// FailOnAny contains conditions to fail on.
type FailOnAny struct {
	// Timeout is how long to wait on this Target before failing.  Defaults to 30 minutes.
	Timeout time.Duration `json:"timeout,omitempty"`

	// Only apply these conditions to Resources of this type from the list of applied Resources.
	// Filter all Resources if unspecified.
	Resource metav1.GroupVersionKind `json:"resource,omitempty"`

	// Selector filters the conditions to a specific set of Resources.  Filter all Resources if unspecified.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Conditions are Conditions for Resources that were provided to Apply.  All matching Resources must
	// have these Conditions.
	Conditions map[string]string `json:"conditions,omitempty"`

	// Logs defines a list of regex to match Pod logs.  Only looks at Pods matching Selector.
	// Will follow logs until this regex is matched.  Requires Selector to be specified, and only
	// matches Pods matching the selector.
	Logs []string `json:"logs,omitempty"`

	// ApplicationMetrics are ApplicationMetrics from Prometheus or Stackdriver to watch for.
	ApplicationMetrics ApplicationMetrics `json:"applicationMetrics,omitempty"`
}

// ApplicationMetrics matches Application Metrics.
type ApplicationMetrics struct {
	// TODO: Write this some day.
}

// Status matches high-level Status for Resources
type Status struct {
	// Healthy waits the resources to be healthy.
	// +optional
	Healthy *bool `json:"healthy,omitempty"`

	// Complete waits the resources to be settled.
	// +optional
	Settled *bool `json:"settled,omitempty"`

	// Complete waits for all of the Pods to exit.
	// +optional
	Complete *bool `json:"complete,omitempty"`
}

// Config configures reading the kubeconfig.
type Config struct {
	// Path is the path to the kubeconfig file.  Defaults to environment variable `KUBECONFIG` or
	// ~/.kube/config (if KUBECONFIG is not set).  Behaves as if the --kubeconfig was set.
	// +optional
	Path *string `json:"path,omitempty"`

	// Context is the kubeconfig context to use.  Defaults to the "current" context.  Behaves as if
	// the --context flag was set.
	// +optional
	Context *string `json:"context,omitempty"`

	// Cluster is the kubeconfig cluster to user.  Behaves as if the --cluster flag was set.
	Cluster *string `json:"cluster,omitempty"`
}
