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

package delete_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestDeleteEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	d, done, err := wiretest.InitializeDelete(clik8s.ResourceConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := d.Do()
	assert.NoError(t, err)
	assert.Equal(t, delete.Result{}, r)
}

func TestDelete(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)
	updatedObjects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	a.Resources = updatedObjects
	_, err = a.Do()
	assert.NoError(t, err)

	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 3)

	d, doned, err := wiretest.InitializeDelete(updatedObjects, &object.Commit{}, buf)
	defer doned()
	assert.NoError(t, err)
	d.DynamicClient = a.DynamicClient
	_, err = d.Do()
	assert.NoError(t, err)

	err = d.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)
}

func InitializeKustomizationWithPresence() ([]string, func(), error) {
	f1, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f1, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

resources:
- not-delete-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f1, "not-delete-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    kubectl.kubernetes.io/presence: PreventDeletion
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	f2, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f2, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map
  literals:
  - foo=bar

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	return []string{f1, f2}, func() {
		os.RemoveAll(f1)
		os.RemoveAll(f2)
	}, nil
}

/* TestDeleteWithPresence verifies that Delete doesn't
   delete resources with PreventDeletion annotation.
   It takes the following steps
   1. create a Kustomization with
         a ConfigMapGenerator
         a Service with the annotation PreventDeletion
         an inventory ConfigMap
   2. apply the kustomization
   3. confirm that there are
         2 ConfigMaps
         1 Service
   3. remove the service resource from the Kustomization
      Update the ConfigmapGeneraotr, which will generate
      a ConfigMap with a different name
   4. apply the kustomization again
   5. confirm that the Service object is not deleted. There are
         3 ConfigMaps
         1 Service
   6. run delete on the updated kustomization
   7. confirm that
         3 ConfigMaps are deleted
         1 Service still exists
*/
func TestDeleteWithPresence(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := InitializeKustomizationWithPresence()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// Setup the first kustomization
	// Confirm that it contains a Service
	// with the Annotation PreventDeletion
	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	service := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/presence": "PreventDeletion",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.Contains(t, objects, service)

	// setup the ConfigMap list and Service list
	// used for confirmation
	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})
	serviceList := &unstructured.UnstructuredList{}
	serviceList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ServiceList",
		Version: "v1",
	})
	defaultCount := 1

	// Apply the first kustomization
	// Confirm that are
	//  2 ConfigMaps
	//  1 Service
	// Confirm that the service is as expected
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)
	err = a.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, defaultCount+1, len(serviceList.Items))
	liveService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
		},
	}
	exist, err := util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))

	// Update the kustomization by
	//     removing the service object
	//     changing the ConfigMapGenerator to make it generate a different ConfigMap
	// Apply it again and Confirm that there are
	//   3 ConfigMaps
	//   1 Service
	updatedObjects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	assert.NotContains(t, updatedObjects, service)

	a.Resources = updatedObjects
	_, err = a.Do()
	assert.NoError(t, err)
	err = a.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 3)
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(serviceList.Items), defaultCount+1)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))

	// Run Delete on the updated kustomization
	// Confirm that
	//   3 ConfigMaps are deleted
	//   the Service object still exists
	d, doned, err := wiretest.InitializeDelete(updatedObjects, &object.Commit{}, buf)
	defer doned()
	assert.NoError(t, err)
	d.DynamicClient = a.DynamicClient
	_, err = d.Do()
	assert.NoError(t, err)
	err = d.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(serviceList.Items), defaultCount+1)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))
}

func InitializeKustomizationWithService() ([]string, func(), error) {
	f1, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f1, "kustomization.yaml"), []byte(`
resources:
- not-delete-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f1, "not-delete-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    kubectl.kubernetes.io/presence: PreventDeletion
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	f2, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f2, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f2, "service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	return []string{f1, f2}, func() {
		os.RemoveAll(f1)
		os.RemoveAll(f2)
	}, nil
}

/* TestDeleteWithPresence verifies that Delete can delete a resource
   when the PreventDeletion annotation is removed.
   It takes the following steps
   1. create a Kustomization with
         a service with annotation PreventDeletion
   2. apply the kustomization
   3. confirm that there are
         1 Service
   4. run delete and confirm the Service is not deleted
   5. update the Service by removing the annotation
   6. apply the kustomization again
   7. confirm that the Service object exists
   8. run delete on the updated kustomization
   9. confirm that
         the Service is deleted
*/
func TestDeleteWithPresenceNoInventory(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := InitializeKustomizationWithService()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// Setup the first kustomization
	// Confirm that it contains a Service
	// with the Annotation PreventDeletion
	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	service := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/presence": "PreventDeletion",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.Contains(t, objects, service)

	// Apply the first kustomization
	// Confirm that the service is created
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)
	liveService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
		},
	}
	exist, err := util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))

	// Run Delete
	// Confirm that the service is not deleted
	d, doned, err := wiretest.InitializeDelete(objects, &object.Commit{}, buf)
	defer doned()
	d.DynamicClient = a.DynamicClient
	assert.NoError(t, err)
	_, err = d.Do()
	assert.NoError(t, err)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))

	// Update the kustomization by
	// removing the annotation PreventDeletion from the Service
	// Confirm that the loaded resources
	// have the new Service
	updatedObjects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	updatedService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.Contains(t, updatedObjects, updatedService)

	// Run Delete on the updated kustomization
	// Confirm that the service is not deleted
	d.Resources = updatedObjects
	_, err = d.Do()
	assert.NoError(t, err)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, true, util.MatchAnnotations(liveService, service.GetAnnotations()))



	// Apply the updated kustomization
	// (The PreventDeletion annotation is removed)
	// Confirm that the Service annotation is removed
	a.Resources = updatedObjects
	_, err = a.Do()
	assert.NoError(t, err)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, false, util.MatchAnnotations(liveService, service.GetAnnotations()))

	// Then run delete
	// Confirm that the service is deleted
    _, err = d.Do()
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, false, exist)
}