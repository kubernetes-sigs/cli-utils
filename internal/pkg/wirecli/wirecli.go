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

package wirecli

import (
	"io"

	"github.com/google/wire"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	"sigs.k8s.io/cli-experimental/internal/pkg/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/prune"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wireconfig"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiregit"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
)

// TODO(Liujingfang1): split into per command wire

// ProviderSet defines dependencies for initializing objects
var ProviderSet = wire.NewSet(
	wirek8s.ProviderSet,
	wiregit.OptionalProviderSet,
	wire.Struct(new(status.Status), "*"),
	wire.Struct(new(apply.Apply), "*"),
	wire.Struct(new(prune.Prune), "*"),
	wire.Struct(new(delete.Delete), "*"),
	NewStatusCommandResult,
	NewApplyCommandResult,
	NewDeleteCommandResult,
	NewPruneCommandResult,
	wireconfig.ConfigProviderSet,
)

// NewStatusCommandResult returns a new status.Result
func NewStatusCommandResult(s *status.Status, out io.Writer) (status.Result, error) {
	return s.Do()
}

// NewApplyCommandResult returns a new apply.Result
func NewApplyCommandResult(a *apply.Apply, out io.Writer) (apply.Result, error) {
	return a.Do()
}

// NewPruneCommandResult returns a new prune.Result
func NewPruneCommandResult(p *prune.Prune, out io.Writer) (prune.Result, error) {
	return p.Do()
}

// NewDeleteCommandResult returns a new delete.Result
func NewDeleteCommandResult(d *delete.Delete, out io.Writer) (delete.Result, error) {
	return d.Do()
}
