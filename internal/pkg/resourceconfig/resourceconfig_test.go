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
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
	"sigs.k8s.io/kustomize/pkg/inventory"
	"sigs.k8s.io/yaml"
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
	pobject, err := kp.GetConfig(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, pobject)
	assert.NotNil(t, pobject)
	inv := inventory.NewInventory()
	inv.LoadFromAnnotation(pobject[1].GetAnnotations())
	assert.Equal(t, len(inv.Current), 1)
}

var (
	serviceA = `
apiVersion: v1
kind: Service
metadata:
  name: service-a
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`
	serviceB = `
apiVersion: v1
kind: Service
metadata:
  name: service-b
  annotations:
    kustomize.config.k8s.io/Inventory: ""
    kustomize.config.k8s.io/InventoryHash: 8mk644dhch
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`
	serviceC = `
apiVersion: v1
kind: Service
metadata:
  name: service-c
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`
	cm = `
apiVersion: v1
kind: ConfigMap
metadata:
 name: cm
`
)

/*
setupRawConfigFiles provides a directory with Kubernetes resources
that can be used as input to test RawConfigFileProvider.
The directory created is

f
├── README.md
├── nonk8s.yaml
├── service.yaml
├── subdir1
│   └── service.yaml
└── subdir2
    └── service.yaml

 */
func setupRawConfigFiles(t *testing.T) (string, string) {
	f, err := ioutil.TempDir("/tmp", "TestConfigProvider")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "README.md"), []byte(`
readme
`), 0644)
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "nonk8s.yaml"), []byte(`
key: value
`), 0644)
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "service.yaml"), []byte(serviceA), 0644)
	assert.NoError(t, err)
	subdir1, err := ioutil.TempDir(f, "subdir")
	assert.NoError(t, err)
	subdir2, err := ioutil.TempDir(f, "subdir")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(subdir1, "service.yaml"), []byte(
		serviceB + "\n---\n" + cm), 0644)
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(subdir2, "service.yaml"), []byte(
		serviceC), 0644)
	assert.NoError(t, err)
	return f, subdir1
}

func setupExpected(t *testing.T) []*unstructured.Unstructured {
	var results []*unstructured.Unstructured
	inputs := []string{serviceA, serviceB, cm, serviceC}
	for _, input := range inputs {
		body := map[string]interface{}{}
		err := yaml.Unmarshal([]byte(input), &body)
		assert.NoError(t, err)
		results = append(results, &unstructured.Unstructured{Object: body})
	}
	return results
}

func TestRawConfigFileProvider(t *testing.T) {
	f, subdir := setupRawConfigFiles(t)
	defer os.RemoveAll(f)
	expected := setupExpected(t)

	cp := wiretest.InitializeRawConfigProvider()
	b := cp.IsSupported(f)
	assert.Equal(t, b, true)
	resources, err := cp.GetConfig(f)
	assert.NoError(t, err)
	assert.Equal(t, len(resources), 4)
	assert.Equal(t, expected, resources)
	resources, err = cp.GetConfig(filepath.Join(f, "service.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, len(resources), 1)
	assert.Equal(t, expected[0], resources[0])

	b = cp.IsSupported(subdir)
	resources, err = cp.GetConfig(subdir)
	assert.NoError(t, err)
	assert.Equal(t, len(resources), 2)
	assert.Equal(t, expected[1:3], resources)
	resources, err = cp.GetConfig(filepath.Join(subdir, "service.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, len(resources), 2)
	assert.Equal(t, expected[1:3], resources)
}
