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

package wiretest

import (
	"github.com/google/wire"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// ProviderSet defines dependencies for initializing objects
var ProviderSet = wire.NewSet(
	dy.ProviderSet, wirek8s.NewKubernetesClientSet, wirek8s.NewExtensionsClientSet, wirek8s.NewDynamicClient,
	NewRestConfig, status.Status{}, apply.Apply{})

// NewRestConfig provides a rest.Config for a testing environment
func NewRestConfig() (*rest.Config, func(), error) {
	e := envtest.Environment{}
	c, err := e.Start()
	return c, func() { e.Stop() }, err
}
