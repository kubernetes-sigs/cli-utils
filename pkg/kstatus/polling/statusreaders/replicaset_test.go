// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
)

func TestReplicaSetStatusReader(t *testing.T) {
	manifest := `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: Bar
spec:
  replicas: 3
  selector:
    matchLabels:
      tier: frontend
`

	podManifest1 := `
apiVersion: v1
kind: Pod
metadata:
  name: Bar-12345
`

	podManifest2 := `
apiVersion: v1
kind: Pod
metadata:
  name: Bar-54321
`

	gvk := v1.SchemeGroupVersion.WithKind("Pod")

	generatedObjects := []unstructured.Unstructured{
		*testutil.YamlToUnstructured(t, podManifest1),
		*testutil.YamlToUnstructured(t, podManifest2),
	}

	var newRsStatusReaderFunc newStatusReaderFunc = func(reader engine.ClusterReader, mapper meta.RESTMapper) engine.StatusReader {
		return NewReplicaSetStatusReader(reader, mapper, &fakeStatusReader{})
	}

	basicStatusReaderTest(t, manifest, gvk, generatedObjects, newRsStatusReaderFunc)
}
