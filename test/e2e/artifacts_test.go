// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"strings"
)

var deployment1 = []byte(strings.TrimSpace(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 4
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.19.6
        ports:
        - containerPort: 80
`))

var apiservice1 = []byte(strings.TrimSpace(`
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1beta1.custom.metrics.k8s.io
spec:
  insecureSkipTLSVerify: true
  group: custom.metrics.k8s.io
  groupPriorityMinimum: 100
  versionPriority: 100
  service:
    name: custom-metrics-stackdriver-adapter
    namespace: custom-metrics
  version: v1beta1
`))

var invalidCrd = []byte(strings.TrimSpace(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: invalidexamples.cli-utils.example.io
spec:
  conversion:
    strategy: None
  group: cli-utils.example.io
  names:
    kind: InvalidExample
    listKind: InvalidExampleList
    plural: invalidexamples
    singular: invalidexample
  scope: Cluster
`))

var pod1 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod1
spec:
  containers:
  - name: kubernetes-pause
    image: registry.k8s.io/pause:2.0
`))

var pod2 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod2
spec:
  containers:
  - name: kubernetes-pause
    image: registry.k8s.io/pause:2.0
`))

var pod3 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod3
spec:
  containers:
  - name: kubernetes-pause
    image: registry.k8s.io/pause:2.0
`))

var podATemplate = `
kind: Pod
apiVersion: v1
metadata:
  name: pod-a
  namespace: {{.Namespace}}
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: Pod
          name: pod-b
          namespace: {{.Namespace}}
        sourcePath: $.status.podIP
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-ip}
      - sourceRef:
          kind: Pod
          name: pod-b
          namespace: {{.Namespace}}
        sourcePath: $.spec.containers[?(@.name=="nginx")].ports[?(@.name=="tcp")].containerPort
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-port}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
    env:
    - name: SERVICE_HOST
      value: "${pob-b-ip}:${pob-b-port}"
`

var podBTemplate = `
kind: Pod
apiVersion: v1
metadata:
  name: pod-b
  namespace: {{.Namespace}}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
`

var invalidMutationPodBTemplate = `
kind: Pod
apiVersion: v1
metadata:
  name: pod-b
  namespace: {{.Namespace}}
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: Pod
          name: pod-a # cyclic dependency
          namespace: {{.Namespace}}
        sourcePath: $.status.podIP
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-ip}
      - sourceRef:
          kind: Pod
          name: pod-a
          namespace: "" # empty namespace on a namespaced type
        sourcePath: $.spec.containers[?(@.name=="nginx")].ports[?(@.name=="tcp")].containerPort
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-port}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
    env:
    - name: SERVICE_HOST
      value: "${pob-b-ip}:${pob-b-port}"
`

var invalidPodTemplate = `
kind: Pod
apiVersion: v1
metadata:
  # missing name
  namespace: {{.Namespace}}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
`

var namespaceTemplate = `
apiVersion: v1
kind: Namespace
metadata:
  name: {{.Namespace}}
`
