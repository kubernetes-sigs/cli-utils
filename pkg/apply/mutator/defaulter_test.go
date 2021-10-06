// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/scheme"
	ktestutil "sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"

	// Using gopkg.in/yaml.v3 instead of sigs.k8s.io/yaml on purpose.
	// yaml.v3 correctly parses ints:
	// https://github.com/kubernetes-sigs/yaml/issues/45
	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaulterTestArtifacts = map[string]string{
	"configmap1": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map1-name
  namespace: map-namespace
data:
  image: traefik/whoami
  version: "1.0"
`,
	"configmap2": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map2-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: "not a valid substitution list"
data: {}
`,
	"configmap3": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map3-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: ConfigMap
          name: map2-name
        sourcePath: $.data
        targetPath: $.data
data: {}
`,
	"configmap4": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: map4-name
  namespace: map-namespace
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: ConfigMap
          name: map2-name
          namespace: map-namespace
        sourcePath: $.data
        targetPath: $.data
data: {}
`,
	"clusterrolebinding1": `
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
`,
	// A ClusterRoleBinding can't technically bind a Role, but the defaulter doesn't know or care.
	"clusterrolebinding2": `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: read-secrets
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          apiVersion: rbac.authorization.k8s.io/v1
          kind: Role
          name: example-role
        sourcePath: $.metadata.labels.domain
        targetPath: $.subjects[0].name
        token: ${domain}
subjects:
- kind: User
  name: "bob@${domain}"
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: secret-reader
  apiGroup: rbac.authorization.k8s.io
`,
	"rolebinding1": `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
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
`,
}

func TestDefaulterMutate(t *testing.T) {
	configmap1 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["configmap1"])
	configmap2 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["configmap2"])
	configmap3 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["configmap3"])
	configmap4 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["configmap4"])
	clusterrolebinding1 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["clusterrolebinding1"])
	clusterrolebinding2 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["clusterrolebinding2"])
	rolebinding1 := ktestutil.YamlToUnstructured(t, defaulterTestArtifacts["rolebinding1"])

	joinedPaths := make([]interface{}, 0)
	err := yaml.Unmarshal([]byte(joinedPathsYaml), &joinedPaths)
	if err != nil {
		t.Fatalf("error parsing yaml: %v", err)
	}

	tests := map[string]struct {
		obj      *unstructured.Unstructured
		mutated  bool
		reason   string
		errMsg   string
		expected *unstructured.Unstructured
	}{
		"no annotation": {
			obj:     configmap1.DeepCopy(),
			mutated: false,
			reason:  "",
		},
		"invalid annotation": {
			obj:     configmap2.DeepCopy(),
			mutated: false,
			reason:  "",
			// exact error message isn't very important. Feel free to update if the error text changes.
			errMsg: `failed to parse apply-time-mutation annotation: "not a valid substitution list": ` +
				`error unmarshaling JSON: while decoding JSON: json: ` +
				`cannot unmarshal string into Go value of type mutation.ApplyTimeMutation`,
			expected: configmap2,
		},
		"cluster-scoped target, cluster-scoped source, no namespace": {
			obj:      clusterrolebinding1.DeepCopy(),
			mutated:  false,
			reason:   "",
			expected: clusterrolebinding1,
		},
		"namespace-scoped target, cluster-scoped source, no namespace": {
			obj:      rolebinding1.DeepCopy(),
			mutated:  false,
			reason:   "",
			expected: rolebinding1,
		},
		"cluster-scoped target, namespace-scoped source, missing namespace": {
			obj:     clusterrolebinding2.DeepCopy(),
			mutated: false,
			reason:  "",
			// exact error message isn't very important. Feel free to update if the error text changes.
			errMsg: `failed to inherit namespace for source resource reference ` +
				`(rbac.authorization.k8s.io/v1/Role/example-role): ` +
				`target resource namespace is empty`,
			expected: clusterrolebinding2,
		},
		"namespace-scoped target, namespace-scoped source, missing namespace": {
			obj:     configmap3.DeepCopy(),
			mutated: true,
			reason: `annotation value updated to inherit namespace ` +
				`(annotation: "config.kubernetes.io/apply-time-mutation", namespace: "map-namespace")`,
			expected: withSourceRefNamespace(t, configmap3.DeepCopy(), 0, configmap3.GetNamespace()),
		},
		"namespace-scoped target, namespace-scoped source, has namespace": {
			obj:      configmap4.DeepCopy(),
			mutated:  false,
			reason:   "",
			expected: configmap4,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mutator := &Defaulter{
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(
					scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...,
				),
			}

			mutated, reason, err := mutator.Mutate(context.TODO(), tc.obj)
			if tc.errMsg != "" {
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.mutated, mutated, "unexpected mutated bool")
			require.Equal(t, tc.reason, reason, "unexpected mutated reason")

			if tc.expected != nil {
				require.Equal(t, tc.expected, tc.obj, "unexpected target field value")
			}
		})
	}
}

func withSourceRefNamespace(t *testing.T, obj *unstructured.Unstructured, sourceIndex int, namespace string) *unstructured.Unstructured {
	a, err := mutation.ReadAnnotation(obj)
	assert.NoError(t, err)
	a[0].SourceRef.Namespace = obj.GetNamespace()
	err = mutation.WriteAnnotation(obj, a)
	assert.NoError(t, err)
	return obj
}
