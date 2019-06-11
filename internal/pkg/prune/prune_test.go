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

package prune_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/prune"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestPruneEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	p, done, err := wiretest.InitializePrune(clik8s.ResourcePruneConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, prune.Result{}, r)
}

/* TestPruneWithoutInventory takes following steps
   1. create a Kustomization with a ConfigMapGenerator and an inventory object
   6. run prune
   7. confirm that no object is pruned since there is no existing inventory object
*/
func TestPruneWithoutInventory(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	assert.NoError(t, err)
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// run the prune
	pruneObject, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 0)
}

/* TestPruneOneObject take following steps
   1. create a Kustomization with a ConfigMapGenerator and an inventory object
   2. apply the kustomization
   3. update the ConfigMapGenerator so that the ConfigMap that gets generated has a different name
   4. apply the kustomization again
   5. confirm that there are 3 ConfigMaps (including the inventroy ConfigMap)
   6. run prune
   7. confirm that there are 2 ConfigMaps (the second ConfigMap and the inventory object)
*/
func TestPruneOneObject(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	assert.NoError(t, err)
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// call apply to create the first configmap
	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)

	// call apply again to create the second configmap
	a.Resources, err = kp.GetConfig(fs[1])
	assert.NoError(t, err)
	_, err = a.Do()
	assert.NoError(t, err)

	// confirm that there are three ConfigMaps
	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 3)

	// run the prune
	pruneObject, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 1)

	// confirm that one ConfigMap is deleted.
	// There are two ConfigMap left.
	err = a.DynamicClient.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)
}

func setupKustomizeWithDeployment(s string) (string, error) {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
`+s+
		`
resources:
- deployment.yaml

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

namespace: default
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "deployment.yaml"), []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  labels:
    app: mysql
spec:
  selector:
    matchLabels:
      app: mysql
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: pass
              key: password
`), 0644)
	if err != nil {
		return "", err
	}

	return f, nil
}

/* TestPruneConfigMapWithDeployment take following steps
   1. create a Kustomization with a SecretGenerator, a Deployment
      that refers to the generated Secret as well as an inventory object
   2. apply the kustomization
   3. update the SecretGenerator so that the Secret that gets generated
      has a different name
   4. apply the kustomization again
   5. confirm that there are 2 Secrets
   6. run prune
   7. confirm that there are 2 Secrets, the first generated Secret is
      not deleted since it is referred by the Deployment and the
      Deployment object is not removed yet.
*/
func TestPruneConfigMapWithDeployment(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()

	// setup the first version of resource configurations
	// and run apply
	f1, err := setupKustomizeWithDeployment(`
secretGenerator:
- name: pass
  literals:
  - password=secret
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f1)
	assert.NoError(t, err)
	objects, err := kp.GetConfig(f1)
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)

	// setup the second version of resource configurations
	// and run apply
	f2, err := setupKustomizeWithDeployment(`
secretGenerator:
- name: pass
  literals:
  - password=secret
  - a=b
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f2)
	a.Resources, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	_, err = a.Do()
	assert.NoError(t, err)

	// Confirm that there are two Secrets
	sList := &unstructured.UnstructuredList{}
	sList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "SecretList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 2)

	// Run prune and assert there are no objects get deleted
	pruneObject, err := kp.GetConfig(f2)
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 0)

	// Confirm that there are two Secrets
	err = a.DynamicClient.List(context.Background(), sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 2)
}

func setupKustomizeWithStatefulSet(s string) (string, error) {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
`+s+
		`
resources:
- statefulset.yaml

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

namespace: default
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "statefulset.yaml"), []byte(`
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: web
spec:
  selector:
    matchLabels:
      app: nginx
  serviceName: "nginx"
  replicas: 3 # by default is 1
  template:
    metadata:
      labels:
        app: nginx
    spec:
      terminationGracePeriodSeconds: 10
      containers:
      - name: nginx
        image: k8s.gcr.io/nginx-slim:0.8
        ports:
        - containerPort: 80
          name: web
        env:
        - name: PASSWORD
          valueFrom:
            secretKeyRef:
              name: pass
              key: password
`), 0644)
	if err != nil {
		return "", err
	}

	return f, nil
}

/* TestPruneConfigMapWithStatefulSet take following steps
   1. create a Kustomization with a SecretGenerator, a StatefulSet
      that refers to the generated Secret as well as an inventory object
   2. apply the kustomization
   3. update the SecretGenerator so that the Secret that gets generated
      has a different name
   4. apply the kustomization again
   5. confirm that there are 2 Secrets
   6. run prune
   7. confirm that there are 2 Secrets, the first generated Secret is
      not deleted since it is referred by the StatefulSet and the
      Deployment object is not removed yet.
*/
func TestPruneConfigMapWithStatefulSet(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()

	// setup the first version of resource configurations
	// and run apply
	f1, err := setupKustomizeWithStatefulSet(`
secretGenerator:
- name: pass
  literals:
  - password=secret
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f1)
	assert.NoError(t, err)
	objects, err := kp.GetConfig(f1)
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)

	// setup the second version of resource configurations
	// and run apply
	f2, err := setupKustomizeWithStatefulSet(`
secretGenerator:
- name: pass
  literals:
  - password=secret
  - a=b
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f2)
	a.Resources, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	_, err = a.Do()
	assert.NoError(t, err)

	// Confirm that there are two Secrets
	sList := &unstructured.UnstructuredList{}
	sList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "SecretList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 2)

	// Run prune and assert there are no objects get deleted
	pruneObject, err := kp.GetConfig(f2)
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 0)

	// Confirm that there are two Secrets
	err = a.DynamicClient.List(context.Background(), sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 2)
}

func setupKustomizeWithMultipleObjects(s string) (string, error) {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
`+s+
		`
resources:
- deployment.yaml
- service.yaml

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

namespace: default
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "deployment.yaml"), []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  labels:
    app: mysql
spec:
  selector:
    matchLabels:
      app: mysql
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: pass
              key: password
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: mysql
  labels:
    app: mysql
  annotations: {}
spec:
  ports:
    - port: 3306
  selector:
    app: mysql
`), 0644)
	if err != nil {
		return "", err
	}

	return f, nil
}

/* TestPruneConfigMapWithMultipleObjects take following steps
   1. create a Kustomization with
         a SecretGenerator
         a Deployment that uses the generated Secret
         a Service
         an inventory ConfigMap
   2. apply the kustomization
   3. update the SecretGenerator so that the Secret that gets generated
      has a different name
   4. add a namePrefix in the kustomization
   5. apply the kustomization again
   6. confirm that there are
         2 Secrets
         2 Deployments
         2 Services
   7. run prune and confirms 3 objects are deleted
   8. confirm that there are
         1 Secret
         1 Deployment
         1 Service
*/
func TestPruneConfigMapWithMultipleObjects(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	ctx := context.Background()

	// setup the first version of resource configurations
	// and run apply
	f1, err := setupKustomizeWithMultipleObjects(`
secretGenerator:
- name: pass
  literals:
  - password=secret
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f1)
	assert.NoError(t, err)
	objects, err := kp.GetConfig(f1)
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()

	svList := &unstructured.UnstructuredList{}
	svList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ServiceList",
		Version: "v1",
	})
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	serviceNumber := len(svList.Items)

	_, err = a.Do()
	assert.NoError(t, err)

	// setup the second version of resource configurations
	// and run apply
	f2, err := setupKustomizeWithMultipleObjects(`
secretGenerator:
- name: pass
  literals:
  - password=secret
  - a=b

namePrefix: test-
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f2)
	a.Resources, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	_, err = a.Do()
	assert.NoError(t, err)

	// Confirm that there are two Secrets
	sList := &unstructured.UnstructuredList{}
	sList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "SecretList",
		Version: "v1",
	})
	err = a.DynamicClient.List(ctx, sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 2)

	// Confirm that there are two Deployments
	dpList := &unstructured.UnstructuredList{}
	dpList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "DeploymentList",
		Version: "v1",
		Group:   "apps",
	})
	err = a.DynamicClient.List(ctx, dpList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(dpList.Items), 2)

	// Confirm that there are two Services
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber+2)

	// Run prune and assert there are 3 objects get deleted
	pruneObject, err := kp.GetConfig(f2)
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 3)

	// Confirm that there are one Secret
	err = a.DynamicClient.List(ctx, sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 1)

	// Confirm that there are one Deployment
	err = a.DynamicClient.List(ctx, dpList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(dpList.Items), 1)

	// Confirm that there are one Service
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber+1)
}

func setupKustomizeWithDeclarativeDeletion(s1, s2 string) (string, error) {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
- service.yaml

secretGenerator:
- name: pass
  literals:
  - password=secret

namespace: default
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "deployment.yaml"), []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  labels:
    app: mysql
` + s2 + `
spec:
  selector:
    matchLabels:
      app: mysql
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: pass
              key: password
`), 0644)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(filepath.Join(f, "service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: mysql
  labels:
    app: mysql
` + s1 + `
spec:
  ports:
    - port: 3306
  selector:
    app: mysql
`), 0644)
	if err != nil {
		return "", err
	}

	return f, nil
}

var (
	service = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "mysql",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "mysql",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port": int64(3306),
					},
				},
				"selector": map[string]interface{}{"app": "mysql"},
			},
		},
	}
	updatedService = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "mysql",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "mysql",
				},
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/presence": "EnsureDoesNotExist",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port": int64(3306),
					},
				},
				"selector": map[string]interface{}{"app": "mysql"},
			},
		},
	}
	deployment =  &unstructured.Unstructured{
		Object:map[string]interface {}{
			"apiVersion":"apps/v1",
			"kind":"Deployment",
			"metadata":map[string]interface {}{
				"labels":map[string]interface {}{"app":"mysql"},
				"name":"mysql",
				"namespace":"default",
			},
			"spec":map[string]interface {}{
				"selector":map[string]interface {}{
					"matchLabels":map[string]interface {}{"app":"mysql"}},
					"strategy":map[string]interface {}{"type":"Recreate"},
					"template":map[string]interface {}{
						"metadata":map[string]interface {}{
							"labels":map[string]interface {}{
								"app":"mysql",
							},
						},
						"spec":map[string]interface {}{
							"containers":[]interface {}{
								map[string]interface {}{
									"env":[]interface {}{
										map[string]interface {}{
											"name":"MYSQL_ROOT_PASSWORD",
											"valueFrom":map[string]interface {}{
												"secretKeyRef":map[string]interface {}{
													"key":"password",
													"name":"pass-dfg7h97cf6",
												},
											},
										},
									},
									"image":"mysql:5.6",
									"name":"mysql",
								},
							},
						},
					},
			},
		},
	}

	updatedDeployment =  &unstructured.Unstructured{
		Object:map[string]interface {}{
			"apiVersion":"apps/v1",
			"kind":"Deployment",
			"metadata":map[string]interface {}{
				"labels":map[string]interface {}{"app":"mysql"},
				"name":"mysql",
				"namespace":"default",
				"annotations": map[string]interface {}{
					"kubectl.kubernetes.io/presence": "EnsureDoesNotExist",
				},
			},
			"spec":map[string]interface {}{
				"selector":map[string]interface {}{
					"matchLabels":map[string]interface {}{"app":"mysql"}},
				"strategy":map[string]interface {}{"type":"Recreate"},
				"template":map[string]interface {}{
					"metadata":map[string]interface {}{
						"labels":map[string]interface {}{
							"app":"mysql",
						},
					},
					"spec":map[string]interface {}{
						"containers":[]interface {}{
							map[string]interface {}{
								"env":[]interface {}{
									map[string]interface {}{
										"name":"MYSQL_ROOT_PASSWORD",
										"valueFrom":map[string]interface {}{
											"secretKeyRef":map[string]interface {}{
												"key":"password",
												"name":"pass-dfg7h97cf6",
											},
										},
									},
								},
								"image":"mysql:5.6",
								"name":"mysql",
							},
						},
					},
				},
			},
		},
	}
)

/* TestPruneWithAnnotations verifies that Prune delete the resource
   with EnsureDoesNotExist annotation.
   It takes the following steps:
   1. create a Kustomization with
         a SecretGenerator
         a Deployment that uses the generated Secret
         a Service

   2. apply the kustomization
   3. confirm that there are
         1 Secret
         1 Deployment
         1 Service
   4. add the following annotation to the Service resource
		 kubectl.kubernetes.io/presence: EnsureDoesNotExist

   4. apply the kustomization again
   6. run prune and confirms 1 objects is deleted
   7. confirm that there are
         1 Secret
         1 Deployment
         0 Service
   8. add the following annotation to the Deployment resource
         kubectl.kubernetes.io/presence: EnsureDoesNotExist
   9. apply the kustomization again
  10. run prune and confirms 1 objects is deleted
  11. confirm that there are
         1 Secret
         0 Deployment
         0 Service
*/
func TestPruneWithAnnotations(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	ctx := context.Background()

	// setup the first version of resource configurations
	// and run apply
	f1, err := setupKustomizeWithDeclarativeDeletion("", "")
	defer os.RemoveAll(f1)
	assert.NoError(t, err)
	objects, err := kp.GetConfig(f1)
	assert.NoError(t, err)
	assert.Contains(t, objects, service)
	assert.Contains(t, objects, deployment)

	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()

	svList := &unstructured.UnstructuredList{}
	svList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ServiceList",
		Version: "v1",
	})
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	serviceNumber := 1

	_, err = a.Do()
	assert.NoError(t, err)

	// Confirm that there is one Service
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber+1)

	// Confirm that there is one Secret
	sList := &unstructured.UnstructuredList{}
	sList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "SecretList",
		Version: "v1",
	})
	err = a.DynamicClient.List(ctx, sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 1)

	// Confirm that there is one Deployment
	dpList := &unstructured.UnstructuredList{}
	dpList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "DeploymentList",
		Version: "v1",
		Group:   "apps",
	})
	err = a.DynamicClient.List(ctx, dpList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(dpList.Items), 1)


	// update the kustomization by adding annotation
	// to the Service resource
	// and run apply
	f2, err := setupKustomizeWithDeclarativeDeletion(`
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
`, "")
	assert.NoError(t, err)
	defer os.RemoveAll(f2)
	a.Resources, err = kp.GetConfig(f2)
	assert.NoError(t, err)
	assert.Contains(t, a.Resources, updatedService)
	assert.Contains(t, a.Resources, deployment)

	_, err = a.Do()
	assert.NoError(t, err)

	// Confirm that there is one Service
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber+1)

	// Run prune and assert there is one object deleted
	pruneObject, err := kp.GetConfig(f2)
	assert.NoError(t, err)
	p, donep, err := wiretest.InitializePrune(pruneObject, &object.Commit{}, buf)
	defer donep()
	assert.NoError(t, err)
	p.DynamicClient = a.DynamicClient
	pr, err := p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 1)

	// Confirm that there are one Secret
	err = a.DynamicClient.List(ctx, sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 1)

	// Confirm that there are one Deployment
	err = a.DynamicClient.List(ctx, dpList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(dpList.Items), 1)

	// Confirm that there is no Service
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber)

    // update the kustomization by adding annotation
    // to the Deployment resource
    // and run apply
	f3, err := setupKustomizeWithDeclarativeDeletion(`
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
`, `
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
`)
	assert.NoError(t, err)
	defer os.RemoveAll(f3)
	a.Resources, err = kp.GetConfig(f3)
	assert.NoError(t, err)
	assert.Contains(t, a.Resources, updatedService)
	assert.Contains(t, a.Resources, updatedDeployment)

	_, err = a.Do()
	assert.NoError(t, err)

	// Run prune and assert there is one object deleted
	pruneObject, err = kp.GetConfig(f3)
	assert.NoError(t, err)
	p.Resources = pruneObject
	pr, err = p.Do()
	assert.NoError(t, err)
	assert.Equal(t, len(pr.Resources), 1)

	// Confirm that there are one Secret
	err = a.DynamicClient.List(ctx, sList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(sList.Items), 1)

	// Confirm that there is no Deployment
	err = a.DynamicClient.List(ctx, dpList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(dpList.Items), 0)

	// Confirm that there is no Service
	err = a.DynamicClient.List(ctx, svList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(svList.Items), serviceNumber)
}
