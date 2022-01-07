// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	ktestutil "sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/object"

	// Using gopkg.in/yaml.v3 instead of sigs.k8s.io/yaml on purpose.
	// yaml.v3 correctly parses ints:
	// https://github.com/kubernetes-sigs/yaml/issues/45
	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/require"
)

var expectedReason = "resource contained annotation: config.kubernetes.io/apply-time-mutation"

var pod1y = `
apiVersion: v1
kind: Pod
metadata:
  name: pod-name
  namespace: pod-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          group: networking.k8s.io
          kind: Ingress
          name: ingress1-name
          namespace: ingress-namespace
        sourcePath: $.spec.rules[0].http.paths[0].backend.service.port.number
        targetPath: $.spec.containers[0].env[0].value
        token: ${service-port}
spec:
  containers:
  - name: app
    image: example:1.0
    ports:
    - containerPort: 80
    env:
    - name: SERVICE_PORT
      value: ${service-port}
`

var ingress1y = `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ingress1-name
  namespace: ingress-namespace
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - http:
      paths:
      - path: /old
        pathType: Prefix
        backend:
          service:
            name: old
            port:
              number: 80
`

var pod2y = `
apiVersion: v1
kind: Pod
metadata:
  name: pod-name
  namespace: pod-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          group: networking.k8s.io
          kind: Ingress
          name: ingress1-name
          namespace: ingress-namespace
        sourcePath: $.spec.rules[?(@.http)].http.paths[?(@.path=="/old")].backend.service.port.number
        targetPath: $.spec.containers[?(@.name=="app")].env[?(@.name=="SERVICE_PORT")].value
        token: ${service-port}
      - sourceRef:
          group: networking.k8s.io
          kind: Ingress
          name: ingress1-name
          namespace: ingress-namespace
        sourcePath: $.spec.rules[?(@.http)].http.paths[?(@.path=="/old")].backend.service.name
        targetPath: $.spec.containers[?(@.name=="app")].env[?(@.name=="SERVICE_NAME")].value
spec:
  containers:
  - name: app
    image: example:1.0
    ports:
    - containerPort: 80
    env:
    - name: SERVICE_PORT
      value: ${service-port}
    - name: SERVICE_NAME
      value: "" # field must exist to be mutated
`

var pod3y = `
apiVersion: v1
kind: Pod
metadata:
  name: pod-name
  namespace: pod-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: ConfigMap
          name: map1-name
          namespace: map-namespace
        sourcePath: $.data.image
        targetPath: $.spec.containers[?(@.name=="app")].image
        token: ${app-image}
      - sourceRef:
          kind: ConfigMap
          name: map1-name
          namespace: map-namespace
        sourcePath: $.data.version
        targetPath: $.spec.containers[?(@.name=="app")].image
        token: ${app-version}
      - sourceRef:
          group: networking.k8s.io
          kind: Ingress
          name: ingress1-name
          namespace: ingress-namespace
        sourcePath: $.spec.rules[?(@.http)].http.paths[?(@.path=="/old")].backend.service.port.number
        targetPath: $.spec.containers[?(@.name=="app")].env[?(@.name=="SERVICE_PORT")].value
        token: ${service-port}
spec:
  containers:
  - name: app
    image: ${app-image}:${app-version}
    ports:
    - containerPort: 80
    env:
    - name: SERVICE_PORT
      value: ${service-port}
`

var configmap1y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
data:
  image: traefik/whoami
  version: "1.0"
`

var configmap2y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map2-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: ConfigMap
          name: map1-name
          namespace: map-namespace
        sourcePath: $.data
        targetPath: $.data.json
        token: ${map-data-json}
data:
  json: "[{\"π\":3.14},${map-data-json}]"
`

// invalid
var configmap3y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map3-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: "not a valid substitution list"
data: {}
`

// self-reference
var configmap4y = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map4-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: ConfigMap
          name: map4-name
          namespace: map-namespace
        sourcePath: $.data
        targetPath: $.data
data:
  movie: inception
  slogan: we need to go deeper
`

var ingress2y = `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ingress2-name
  namespace: ingress-namespace
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          apiVersion: networking.k8s.io/v1
          kind: Ingress
          name: ingress1-name
          namespace: ingress-namespace
        sourcePath: $.spec.rules[0].http.paths[?(@.path=="/old")]
        targetPath: $.spec.rules[0].http.paths[(@.length-1)]
spec:
  rules:
  - http:
      paths:
      - path: /new
        pathType: Prefix
        backend:
          service:
            name: new
            port:
              number: 80
      - {} # field must exist to be mutated
`

var joinedPathsYaml = `
- path: /new
  pathType: Prefix
  backend:
    service:
      name: new
      port:
        number: 80
- path: /old
  pathType: Prefix
  backend:
    service:
      name: old
      port:
        number: 80
`

var service1y = `
apiVersion: v1
kind: Service
metadata:
  name: service1-name
  namespace: service1-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          group: apps
          kind: Deployment
          name: deployment1-name
          namespace: deployment1-namespace
        sourcePath: $.spec.template.spec.containers[?(@.name=="tcp-handler")].ports[0].containerPort
        targetPath: $.spec.ports[?(@.protocol=="TCP" && @.port==80)].targetPort
      - sourceRef:
          group: apps
          kind: Deployment
          name: deployment1-name
          namespace: deployment1-namespace
        sourcePath: $.spec.template.spec.containers[?(@.name=="udp-handler")].ports[0].containerPort
        targetPath: $.spec.ports[?(@.protocol=="UDP" && @.port==80)].targetPort
spec:
  selector:
    app: MyApp
  ports:
    - protocol: TCP
      port: 80
      targetPort: 0 # field must exist to be mutated
    - protocol: TCP
      port: 443
      targetPort: 443
    - protocol: UDP
      port: 80
      targetPort: 0 # field must exist to be mutated
`

var deployment1y = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment1-name
  namespace: deployment1-namespace
spec:
  selector:
    matchLabels:
      app: example
  replicas: 2
  template:
    metadata:
      labels:
        app: example
    spec:
      containers:
      - name: tcp-handler
        image: example-tcp
        ports:
        - containerPort: 8080
      - name: udp-handler
        image: example-udp
        ports:
        - containerPort: 8081
`

var clusterrole1y = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: example-role
  labels:
    domain: example.com
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list"]
`

var clusterrolebinding1y = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: read-secrets
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          apiVersion: rbac.authorization.k8s.io/v1
          kind: ClusterRole
          name: example-role
        sourcePath: $.metadata.labels.domain
        targetPath: $.subjects[0].name
        token: ${domain}
subjects:
- kind: User
  name: "bob@${domain}"
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: secret-reader
  apiGroup: rbac.authorization.k8s.io
`

type nestedFieldValue struct {
	Field []interface{}
	Value interface{}
}

func TestMutate(t *testing.T) {
	pod1 := ktestutil.YamlToUnstructured(t, pod1y)
	ingress1 := ktestutil.YamlToUnstructured(t, ingress1y)
	pod2 := ktestutil.YamlToUnstructured(t, pod2y)
	pod3 := ktestutil.YamlToUnstructured(t, pod3y)
	configmap1 := ktestutil.YamlToUnstructured(t, configmap1y)
	configmap2 := ktestutil.YamlToUnstructured(t, configmap2y)
	configmap3 := ktestutil.YamlToUnstructured(t, configmap3y)
	configmap4 := ktestutil.YamlToUnstructured(t, configmap4y)
	ingress2 := ktestutil.YamlToUnstructured(t, ingress2y)
	service1 := ktestutil.YamlToUnstructured(t, service1y)
	deployment1 := ktestutil.YamlToUnstructured(t, deployment1y)
	clusterrole1 := ktestutil.YamlToUnstructured(t, clusterrole1y)
	clusterrolebinding1 := ktestutil.YamlToUnstructured(t, clusterrolebinding1y)

	joinedPaths := make([]interface{}, 0)
	err := yaml.Unmarshal([]byte(joinedPathsYaml), &joinedPaths)
	if err != nil {
		t.Fatalf("error parsing yaml: %v", err)
	}

	tests := map[string]struct {
		target   *unstructured.Unstructured
		sources  []*unstructured.Unstructured
		cache    cache.ResourceCache
		mutated  bool
		reason   string
		errMsg   string
		expected []nestedFieldValue
	}{
		"no annotation": {
			target:  configmap1,
			mutated: false,
			reason:  "",
		},
		"invalid annotation": {
			target:  configmap3,
			mutated: false,
			reason:  "",
			// exact error message isn't very important. Feel free to update if the error text changes.
			errMsg: `failed to read annotation in resource (v1/namespaces/map-namespace/ConfigMap/map3-name): ` +
				`failed to parse apply-time-mutation annotation: "not a valid substitution list": ` +
				`error unmarshaling JSON: ` +
				`while decoding JSON: ` +
				`json: cannot unmarshal string into Go value of type mutation.ApplyTimeMutation`,
		},
		"invalid self-reference": {
			target:  configmap4,
			mutated: false,
			reason:  "",
			// exact error message isn't very important. Feel free to update if the error text changes.
			errMsg: `invalid self-reference (/namespaces/map-namespace/ConfigMap/map4-name)`,
		},
		"missing source": {
			target:  pod1,
			mutated: false,
			reason:  "",
			// exact error message isn't very important. Feel free to update if the error text changes.
			errMsg: `failed to get source resource (networking.k8s.io/namespaces/ingress-namespace/Ingress/ingress1-name): ` +
				`resource not found: ` +
				`ingresses.networking.k8s.io "ingress1-name" not found`,
		},
		"pod env var string from ingress port int": {
			target:  pod1,
			sources: []*unstructured.Unstructured{ingress1},
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "containers", 0, "env", 0, "value"},
					Value: "80", // must be string, not int
				},
			},
		},
		"two subs, one source, no token, missing target field, field selector": {
			target:  pod2,
			sources: []*unstructured.Unstructured{ingress1, ingress1}, // twice, because not cached
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "containers", 0, "env", 0, "value"},
					Value: "80", // must be string, not int
				},
				{
					Field: []interface{}{"spec", "containers", 0, "env", 1, "value"},
					Value: "old",
				},
			},
		},
		"two subs, one source, no token, missing target field, field selector (cached)": {
			target:  pod2,
			sources: []*unstructured.Unstructured{ingress1}, // only once, because cached
			cache:   cache.NewResourceCacheMap(),
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "containers", 0, "env", 0, "value"},
					Value: "80", // must be string, not int
				},
				{
					Field: []interface{}{"spec", "containers", 0, "env", 1, "value"},
					Value: "old",
				},
			},
		},
		"three subs, two sources, two tokens in the same target field, float string": {
			target:  pod3,
			sources: []*unstructured.Unstructured{configmap1, configmap1, ingress1}, // repeats, because not cached
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "containers", 0, "env", 0, "value"},
					Value: "80", // must be string, not int
				},
				{
					Field: []interface{}{"spec", "containers", 0, "image"},
					Value: "traefik/whoami:1.0", // make sure float string isn't trucated to "1"
				},
			},
		},
		"three subs, two sources, two tokens in the same target field, float string (cached)": {
			target:  pod3,
			sources: []*unstructured.Unstructured{configmap1, ingress1}, // no repeats, because cached
			cache:   cache.NewResourceCacheMap(),
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "containers", 0, "env", 0, "value"},
					Value: "80", // must be string, not int
				},
				{
					Field: []interface{}{"spec", "containers", 0, "image"},
					Value: "traefik/whoami:1.0", // make sure float string isn't trucated to "1"
				},
			},
		},
		"map to json string": {
			target:  configmap2,
			sources: []*unstructured.Unstructured{configmap1},
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"data", "json"},
					Value: `[{"π":3.14},{"image":"traefik/whoami","version":"1.0"}]`, // string, not object
				},
			},
		},
		"map to map, array append": {
			target:  ingress2,
			sources: []*unstructured.Unstructured{ingress1},
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "rules", 0, "http", "paths"},
					Value: joinedPaths, // object, not string
				},
			},
		},
		"multi-field selector": {
			target:  service1,
			sources: []*unstructured.Unstructured{deployment1, deployment1}, // repeats, because not cached
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"spec", "ports", 0, "targetPort"},
					Value: 8080,
				},
				{
					Field: []interface{}{"spec", "ports", 2, "targetPort"},
					Value: 8081,
				},
			},
		},
		"cluster-scoped": {
			target:  clusterrolebinding1,
			sources: []*unstructured.Unstructured{clusterrole1},
			mutated: true,
			reason:  expectedReason,
			expected: []nestedFieldValue{
				{
					Field: []interface{}{"subjects", 0, "name"},
					Value: "bob@example.com",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			getChan := make(chan unstructured.Unstructured)

			mutator := &ApplyTimeMutator{
				Client: &fakeDynamicClient{
					resourceInterfaceFunc: newFakeNamespaceClientFunc(getChan),
				},
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(
					scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...,
				),
				ResourceCache: tc.cache, // optional!
			}

			// send sources when GET is called
			sources := tc.sources
			go func() {
				defer close(getChan)
				for _, source := range sources {
					getChan <- *source
				}
			}()

			mutated, reason, err := mutator.Mutate(context.TODO(), tc.target)
			if tc.errMsg != "" {
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.mutated, mutated, "unexpected mutated bool")
			require.Equal(t, tc.reason, reason, "unexpected mutated reason")

			for _, efv := range tc.expected {
				received, found, err := object.NestedField(tc.target.Object, efv.Field...)
				require.NoError(t, err)
				require.True(t, found, "target field not found")
				require.Equal(t, efv.Value, received, "unexpected target field value")
			}
		})
	}
}

func TestValueToString(t *testing.T) {
	tests := map[string]struct {
		value    interface{}
		expected string
	}{
		"int": {
			value:    1,
			expected: "1",
		},
		"float": {
			value:    1.2345,
			expected: "1.2345",
		},
		"string": {
			value:    "nothing to see",
			expected: "nothing to see",
		},
		"bool": {
			value:    false,
			expected: "false",
		},
		"interface map": {
			value: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "pod-name",
					"namespace": "test-namespace",
				},
			},
			expected: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod-name","namespace":"test-namespace"}}`,
		},
		"interface list": {
			value: []interface{}{
				"x",
				map[string]interface{}{
					"?": nil,
				},
				0,
			},
			expected: `["x",{"?":null},0]`,
		},
		"string list": {
			value: []string{
				"x",
				"y",
				"z",
			},
			expected: `["x","y","z"]`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			received, err := valueToString(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.expected, received, "unexpected result")
		})
	}
}

// fakeNamespaceClient wraps ResourceInterface, overwriting the Get func.
type fakeNamespaceClient struct {
	dynamic.ResourceInterface
	resource  schema.GroupVersionResource
	namespace string
	getChan   <-chan unstructured.Unstructured
}

func newFakeNamespaceClientFunc(getChan <-chan unstructured.Unstructured) func(resource schema.GroupVersionResource, namespace string) dynamic.ResourceInterface {
	innerGetChan := getChan
	return func(resource schema.GroupVersionResource, namespace string) dynamic.ResourceInterface {
		return &fakeNamespaceClient{
			resource:  resource,
			namespace: namespace,
			getChan:   innerGetChan,
		}
	}
}

func (c *fakeNamespaceClient) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	obj, open := <-c.getChan
	if !open {
		return nil, apierrors.NewNotFound(c.resource.GroupResource(), name)
	}
	return &obj, nil
}

// fakeDynamicClient accepts always returns the same client, just with a different
type fakeDynamicClient struct {
	resourceInterfaceFunc func(resource schema.GroupVersionResource, namespace string) dynamic.ResourceInterface
}

func (c *fakeDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &fakeDynamicResourceClient{
		resourceInterfaceFunc:          c.resourceInterfaceFunc,
		NamespaceableResourceInterface: fake.NewSimpleDynamicClient(scheme.Scheme).Resource(resource),
		resource:                       resource,
	}
}

type fakeDynamicResourceClient struct {
	dynamic.NamespaceableResourceInterface
	resourceInterfaceFunc func(resource schema.GroupVersionResource, namespace string) dynamic.ResourceInterface
	resource              schema.GroupVersionResource
}

func (c *fakeDynamicResourceClient) Namespace(ns string) dynamic.ResourceInterface {
	return c.resourceInterfaceFunc(c.resource, ns)
}
