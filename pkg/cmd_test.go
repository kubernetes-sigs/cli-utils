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

package pkg

import (
	"bytes"
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

func TestCmdWithEmptyInput(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd, done, err := InitializeFakeCmd(buf, nil)
	defer done()
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	err = cmd.Apply(nil)
	assert.NoError(t, err)

	err = cmd.Prune(nil)
	assert.NoError(t, err)

	err = cmd.Delete(nil)
	assert.NoError(t, err)
}

// setupResourcesV1 create a slice of unstructured
// with two ConfigMaps
// 	one with the inventory annotation
// 	one without the inventory annotation
func setupResourcesV1() []*unstructured.Unstructured {
	r1 := &unstructured.Unstructured{}
	r1.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r1.SetName("cm1")
	r1.SetNamespace("default")
	r2 := &unstructured.Unstructured{}
	r2.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r2.SetName("inventory")
	r2.SetNamespace("default")
	r2.SetAnnotations(map[string]string{
		inventory.ContentAnnotation:     "{\"current\":{\"~G_v1_ConfigMap|default|cm1\":null}}",
		inventory.HashAnnotation: "1234567",
	})
	return []*unstructured.Unstructured{r1, r2}
}

// setupResourcesV2 create a slice of unstructured
// with two ConfigMaps
// 	one with the inventory annotation
// 	one without the inventory annotation
func setupResourcesV2() []*unstructured.Unstructured {
	r1 := &unstructured.Unstructured{}
	r1.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r1.SetName("cm2")
	r1.SetNamespace("default")
	r2 := &unstructured.Unstructured{}
	r2.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r2.SetName("inventory")
	r2.SetNamespace("default")
	r2.SetAnnotations(map[string]string{
		inventory.ContentAnnotation:     "{\"current\":{\"~G_v1_ConfigMap|default|cm2\":null}}",
		inventory.HashAnnotation: "7654321",
	})
	return []*unstructured.Unstructured{r1, r2}
}

/* TestCmd tests Apply/Prune/Delete functions by
taking the following steps:
	1. Initialize a Cmd object
	2. Create the Resources v1
	3. Check that there no existing ConfigMap.

	Call apply and prune for the first version of Configs
	4. Apply the resources and confirm that there are 2 ConfigMaps
	5. Prune the resources and confirm that there are 2 ConfigMaps

	Call apply and prune for the second version of Configs
	6. Create the Resources v2
	7. Apply the resources and confirm that there are 3 ConfigMaps
	8. Prune the resources and confirm that there are 2 ConfigMaps

	Cleanup
	9. Delete the resources and confirm that there is no ConfigMap
*/
func TestCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd, done, err := InitializeFakeCmd(buf, nil)
	defer done()
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})

	c := cmd.Applier.DynamicClient
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)

	resources := setupResourcesV1()
	err = cmd.Apply(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	err = cmd.Prune(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	resources = setupResourcesV2()
	err = cmd.Apply(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 3)

	err = cmd.Prune(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	err = cmd.Status(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	err = cmd.Delete(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)
}

func setupResourcesCRD() []*unstructured.Unstructured {
	r1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1beta1",
			"kind": "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "applications.app.k8s.io",
			},
			"spec": map[string]interface{}{
				"group": "app.k8s.io",
				"versions": []interface{}{
					map[string]interface{}{
						"name": "v1beta1",
						"served": true,
						"storage": true,
					},
				},
				"scope": "Namespaced",
				"names": map[string]interface{}{
					"plural": "applications",
					"singular": "application",
					"kind": "Application",
				},
			},
		},
	}
	return []*unstructured.Unstructured{r1}
}

func setupResourcesCR() []*unstructured.Unstructured {
	r := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "app.k8s.io/v1beta1",
			"kind": "Application",
			"metadata": map[string]interface{}{
				"name": "demo-app",
				"namespace": "kalm-e2e",
			},
			"spec": map[string]interface{}{
				"uri": "some uri",
				"Command": "some command",
				"image": "my-image",
			},
		},
	}
	return []*unstructured.Unstructured{r}
}

func setupNamespace() []*unstructured.Unstructured {
	namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "kalm-e2e",
			},
		},
	}
	return []*unstructured.Unstructured{namespace}
}

func TestCmdForCRD(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd, done, err := InitializeFakeCmd(buf, nil)
	defer done()
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	crdList := &unstructured.UnstructuredList{}
	crdList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "CustomResourceDefinitionList",
		Version: "v1beta1",
		Group: "apiextensions.k8s.io",
	})

	c := cmd.Applier.DynamicClient
	err = c.List(context.Background(), crdList, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(crdList.Items), 0)

	resources := setupResourcesCRD()
	err = cmd.Apply(resources)
	assert.NoError(t, err)

	err = c.List(context.Background(), crdList, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(crdList.Items), 1)
	fmt.Println("Successfully installed the CRD")

	namespaces := setupNamespace()
	err = cmd.Apply(namespaces)
	assert.NoError(t, err)

	resources = setupResourcesCR()
	err = cmd.Apply(resources)
	assert.NoError(t, err)

	// make sure the Application CR is successfully applied
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "app.k8s.io/v1beta1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name": "demo-app",
				"namespace": "kalm-e2e",
			},
		},
	}
	err = cmd.Applier.DynamicClient.Get(context.TODO(), types.NamespacedName{
		Name: "demo-app",
		Namespace: "kalm-e2e",
	}, obj)
	if err != nil {
		t.Fatalf("not able to get the application %v", err)
	}
	fmt.Println("Successfully created the application")
}
