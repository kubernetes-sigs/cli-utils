// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"sort"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
)

var configMapObj = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "the-map",
			"namespace": "testspace",
		},
	},
}

var namespaceObj = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "testspace",
		},
	},
}

var deploymentObj = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "testdeployment",
			"namespace": "testspace",
		},
	},
}

var deploymentObj2 = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "testdeployment2",
			"namespace": "testspace",
		},
	},
}

func TestResourceOrdering(t *testing.T) {
	configMapInfo := resource.Info{
		Name:   "the-map",
		Object: &configMapObj,
	}

	namespaceInfo := resource.Info{
		Name:   "testspace",
		Object: &namespaceObj,
	}

	deploymentInfo := resource.Info{
		Name:   "testdeployment",
		Object: &deploymentObj,
	}

	deploymentInfo2 := resource.Info{
		Name:   "testdeployment2",
		Object: &deploymentObj2,
	}

	infos := []*resource.Info{&deploymentInfo, &configMapInfo, &namespaceInfo, &deploymentInfo2}
	sort.Sort(ResourceInfos(infos))

	assert.Equal(t, infos[0].Name, "testspace")
	assert.Equal(t, infos[1].Name, "the-map")
	assert.Equal(t, infos[2].Name, "testdeployment")
	assert.Equal(t, infos[3].Name, "testdeployment2")

	assert.Equal(t, infos[0].Object.GetObjectKind().GroupVersionKind().Kind, "Namespace")
	assert.Equal(t, infos[1].Object.GetObjectKind().GroupVersionKind().Kind, "ConfigMap")
	assert.Equal(t, infos[2].Object.GetObjectKind().GroupVersionKind().Kind, "Deployment")
	assert.Equal(t, infos[3].Object.GetObjectKind().GroupVersionKind().Kind, "Deployment")
}

func TestGvkLessThan(t *testing.T) {
	gvk1 := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	gvk2 := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	}

	assert.Equal(t, IsLessThan(gvk1, gvk2), false)
}

func TestGvkEquals(t *testing.T) {
	gvk1 := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	gvk2 := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	assert.Equal(t, Equals(gvk1, gvk2), true)
}
