// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stress

var namespaceYaml = `
apiVersion: v1
kind: Namespace
metadata:
  name: ""
`

var configMapYaml = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: ""
  namespace: ""
data: {}
`

var cronTabCRDYaml = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: crontabs.stable.example.com
spec:
  group: stable.example.com
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            type: object
            properties:
              cronSpec:
                type: string
              image:
                type: string
  scope: Namespaced
  names:
    plural: crontabs
    singular: crontab
    kind: CronTab
    shortNames:
    - ct
`

var cronTabYaml = `
apiVersion: stable.example.com/v1
kind: CronTab
metadata:
  name: ""
  namespace: ""
spec:
  cronSpec: "* * * * */5"
`

var deploymentYaml = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ""
  namespace: ""
spec:
  replicas: 1
  selector:
    matchLabels:
      app: pause
  template:
    metadata:
      labels:
        app: pause
    spec:
      containers:
      - name: kubernetes-pause
        image: registry.k8s.io/pause:2.0
        resources:
          requests:
            cpu: 1m
            memory: 1Mi
`
