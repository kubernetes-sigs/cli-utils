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

package wirestatus

import (
	"io"

	"github.com/google/wire"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wireconfig"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiregit"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
)

// ProviderSet defines dependencies for initializing objects
var ProviderSet = wire.NewSet(
	wirek8s.ProviderSet,
	wiregit.OptionalProviderSet,
	wire.Struct(new(status.Status), "*"),
	NewStatusCommandResult,
	wireconfig.ConfigProviderSet,
)

// NewStatusCommandResult returns a new status.Result
func NewStatusCommandResult(s *status.Status, out io.Writer) (status.Result, error) {
	return s.Do(), nil
}
