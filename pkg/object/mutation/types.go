// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

// ApplyTimeMutation is a list of substitutions to perform in the target
// object before applying, after waiting for the source objects to be
// reconciled.
// This most notibly allows status fields to be substituted into spec fields.
type ApplyTimeMutation []FieldSubstitution

// Equal returns true if the substitutions are equivalent, ignoring order.
// Fulfills Equal interface from github.com/google/go-cmp
func (a ApplyTimeMutation) Equal(b ApplyTimeMutation) bool {
	if len(a) != len(b) {
		return false
	}

	mapA := make(map[FieldSubstitution]struct{}, len(a))
	for _, sub := range a {
		mapA[sub] = struct{}{}
	}
	mapB := make(map[FieldSubstitution]struct{}, len(b))
	for _, sub := range b {
		mapB[sub] = struct{}{}
	}
	if len(mapA) != len(mapB) {
		return false
	}
	for b := range mapB {
		if _, exists := mapA[b]; !exists {
			return false
		}
	}
	return true
}

// FieldSubstitution specifies a substitution that will be performed at
// apply-time. The source object field will be read and substituted into the
// target object field, replacing the token.
type FieldSubstitution struct {
	// SourceRef is a reference to the object that contains the source field.
	SourceRef reference.ObjectReference `json:"sourceRef"`

	// SourcePath is a JSONPath reference to a field in the source object.
	// Example: "$.status.number"
	SourcePath string `json:"sourcePath"`

	// TargetPath is a JSONPath reference to a field in the target object.
	// Example: "$.spec.member"
	TargetPath string `json:"targetPath"`

	// Token is the substring to replace in the value of the target field.
	// If empty, the target field value will be set to the source field value.
	// Example: "${project-number}"
	// +optional
	Token string `json:"token,omitempty"`
}
