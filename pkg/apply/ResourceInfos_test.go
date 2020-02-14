// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"
	"sort"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	infos := []*resource.Info{&deploymentInfo, &configMapInfo, &namespaceInfo}
	sort.Sort(ResourceInfos(infos))

	var result string
	for _, info := range infos {
		result += fmt.Sprintf("Name: %s, Kind: %s\n", info.Name, info.Object.GetObjectKind().GroupVersionKind().Kind)
	}

	expected := `Name: testspace, Kind: Namespace
Name: the-map, Kind: ConfigMap
Name: testdeployment, Kind: Deployment
`

	assert.Equal(t, result, expected)
}

func TestGvkLessThan(t *testing.T) {
	gvk1 := Gvk{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	gvk2 := Gvk{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	}

	assert.Equal(t, gvk1.IsLessThan(gvk2), false)
}

func TestGvkEquals(t *testing.T) {
	gvk1 := Gvk{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	gvk2 := Gvk{
		Group:   "",
		Version: "v1",
		Kind:    "Deployment",
	}

	assert.Equal(t, gvk1.Equals(gvk2), true)
}
