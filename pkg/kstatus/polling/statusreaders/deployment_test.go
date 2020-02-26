// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
)

func TestDeploymentStatusReader(t *testing.T) {
	deploymentManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
`

	replicaSetManifest1 := `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: Foo-12345
spec:
  replicas: 1
`

	replicaSetManifest2 := `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: Foo-54321
spec:
  replicas: 14
`

	replicaSetGVK := appsv1.SchemeGroupVersion.WithKind("ReplicaSet")

	generatedObjects := []unstructured.Unstructured{
		*testutil.YamlToUnstructured(t, replicaSetManifest1),
		*testutil.YamlToUnstructured(t, replicaSetManifest2),
	}

	var newRsStatusReaderFunc newStatusReaderFunc = func(reader engine.ClusterReader, mapper meta.RESTMapper) engine.StatusReader {
		return NewDeploymentResourceReader(reader, mapper, &fakeStatusReader{})
	}

	basicStatusReaderTest(t, deploymentManifest, replicaSetGVK, generatedObjects, newRsStatusReaderFunc)
}
