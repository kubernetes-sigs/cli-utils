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

package resourceconfig_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/resourceconfig"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

func TestKustomizeProvider(t *testing.T) {
	kp := wiretest.InitializConfigProvider()
	objects, err := kp.GetConfig("github.com/kubernetes-sigs/kustomize//examples/multibases")
	assert.NoError(t, err)
	assert.NotEmpty(t, objects)
	assert.Equal(t, len(objects), 3)
}

func setupKustomize(t *testing.T) string {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`
configMapGenerator:
- name: testmap
namespace: default
inventory:
  type: ConfigMap
  configMap:
    name: root-cm
    namespace: default
`), 0644)
	assert.NoError(t, err)
	return f
}

func TestKustomizeProvider2(t *testing.T) {
	f := setupKustomize(t)
	kp := wiretest.InitializConfigProvider()
	objects, err := kp.GetConfig(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, objects)
	assert.Equal(t, len(objects), 2)
	pobject, err := kp.GetPruneConfig(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, pobject)
	assert.NotNil(t, pobject)
	inv := inventory.NewInventory()
	inv.LoadFromAnnotation(pobject.GetAnnotations())
	assert.Equal(t, len(inv.Current), 1)
}

func setupKustomizeWithoutInventory(t *testing.T) string {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`
configMapGenerator:
- name: testmap
  literals:
  - foo=bar

secretGenerator:
- name: testsc
  literals:
  - bar=baz

namespace: default
`), 0644)
	assert.NoError(t, err)
	return f
}

func TestGetPruneResources(t *testing.T) {
	// with one inventory object
	// GetPruneResources can return it
	f := setupKustomize(t)
	defer os.RemoveAll(f)
	kp := wiretest.InitializConfigProvider()
	objects, err := kp.GetConfig(f)
	assert.NoError(t, err)
	assert.Equal(t, len(objects), 2)

	r, err := resourceconfig.GetPruneResources(objects)
	assert.NoError(t, err)
	assert.NotNil(t, r)

	// Test the empty input
	r, err = resourceconfig.GetPruneResources(
		[]*unstructured.Unstructured{})
	assert.NoError(t, err)
	assert.Nil(t, r)

	// Test the nil input
	r, err = resourceconfig.GetPruneResources(nil)
	assert.NoError(t, err)
	assert.Nil(t, r)

	// With no inventory object
	// GetPruneResources returns nil
	f2 := setupKustomizeWithoutInventory(t)
	defer os.RemoveAll(f2)
	kp = wiretest.InitializConfigProvider()
	objects, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	assert.Equal(t, len(objects), 2)
	r, err = resourceconfig.GetPruneResources(objects)
	assert.NoError(t, err)
	assert.Nil(t, r)

	// With multiple objects with inventory annotations
	// GetPruneResources returns an error
	objects, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	assert.Equal(t, len(objects), 2)
	for _, o := range objects {
		o.SetAnnotations(map[string]string{
			inventory.InventoryHashAnnotation: "12345",
			inventory.InventoryAnnotation:     `{"current": {}}`,
		})
	}
	r, err = resourceconfig.GetPruneResources(objects)
	assert.Errorf(t, err,
		"found multiple resources with inventory annotations")
	assert.Nil(t, r)
}
