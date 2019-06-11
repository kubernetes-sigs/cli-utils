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

package constants

const (
	Presence = "kubectl.kubernetes.io/presence"

	// Any resource with the annotation
	//   kubectl.kubernetes.io/presence: EnsureExist
	// will not be pruned or deleted.
	//
	// It following effect in each command
	// - no effect in apply
	// - prune skips this resource
	// - delete skips this resource

	EnsureExist = "EnsureExist"

	// Any resource with the annotation
	//  kubectl.kubernetes.io/presence: EnsureDoesNotExist
	// Will be deleted or skipped in apply.
	//
	// It has following effect in each command
	// - the resource is skipped in apply
	// - the resource is deleted in prune
	// - the resource is deleted in delete
	EnsureNoExist = "EnsureDoesNotExist"
)