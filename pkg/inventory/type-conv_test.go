// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestObjectReferenceFromObjMetadata(t *testing.T) {
	id := object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "nginx",
		Namespace: "default",
	}

	ref := ObjectReferenceFromObjMetadata(id)

	require.Equal(t, actuation.ObjectReference{
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "nginx",
		Namespace: "default",
	}, ref)
}

func TestObjectReferenceFromObjMetadataClusterScoped(t *testing.T) {
	id := object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "",
			Kind:  "Namespace",
		},
		Name: "kube-system",
	}

	ref := ObjectReferenceFromObjMetadata(id)

	require.Equal(t, actuation.ObjectReference{
		Group: "",
		Kind:  "Namespace",
		Name:  "kube-system",
	}, ref)
}

func TestObjMetadataFromObjectReference(t *testing.T) {
	ref := actuation.ObjectReference{
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "nginx",
		Namespace: "default",
	}

	id := ObjMetadataFromObjectReference(ref)

	require.Equal(t, object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "nginx",
		Namespace: "default",
	}, id)
}

func TestObjectReferenceRoundTrip(t *testing.T) {
	id := object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "rbac.authorization.k8s.io",
			Kind:  "ClusterRole",
		},
		Name:      "admin",
		Namespace: "",
	}

	require.Equal(t, id, ObjMetadataFromObjectReference(ObjectReferenceFromObjMetadata(id)))
}
