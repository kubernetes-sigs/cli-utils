//+build wireinject

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

package wiredelete

import (
	"io"

	"sigs.k8s.io/cli-experimental/internal/pkg/util"

	"github.com/google/wire"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/delete"
)

// DoDelete creates a new Delete object and runs it
func DoDelete(clik8s.ResourceConfigPath, io.Writer, util.Args) (delete.Result, error) {
	panic(wire.Build(ProviderSet))
}
