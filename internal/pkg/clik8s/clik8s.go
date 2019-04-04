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

package clik8s

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceConfigPath is a path containing Kubernetes Resource Config.
//
// Must be one of:
// - A Directory containing a kustomization.yaml
// - A File containing YAML Resource Config (1 or more Resources, separated by `---`)
// - A File containing JSON Resource Config
type ResourceConfigPath string

// KubeConfigPath defines a path to a kubeconfig file used to configure Kubernetes clients.
type KubeConfigPath string

// MasterURL defines the apiserver master url.
type MasterURL string

// ResourceConfigs is a collection of Resource Config read from Files.
type ResourceConfigs []runtime.Object
