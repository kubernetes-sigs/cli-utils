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

package delete_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-experimental/cmd/apply"
	"sigs.k8s.io/cli-experimental/cmd/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

var host string

func TestMain(m *testing.M) {
	c, stop, err := wiretest.NewRestConfig()
	if err != nil {
		os.Exit(1)
	}
	defer stop()
	host = c.Host
	os.Exit(m.Run())
}

func TestDelete(t *testing.T) {
	buf := new(bytes.Buffer)
	fs, cleanup, err := wiretest.InitializeKustomization()
	defer cleanup()
	assert.NoError(t, err)
	assert.NotEmpty(t, fs)

	args := []string{fmt.Sprintf("--server=%s", host), "--namespace=default", fs[0]}
	cmd := apply.GetApplyCommand(args)
	cmd.SetOutput(buf)
	cmd.SetArgs(args)
	wirek8s.Flags(cmd.PersistentFlags())

	assert.NoError(t, cmd.Execute())
	assert.Equal(t, "Doing `cli-experimental apply`\napplied ConfigMap/inventory\napplied ConfigMap/test-map-k6tb869f64\nResources: 2\n", buf.String()) // nolint

	cmd = delete.GetDeleteCommand(args)
	buf.Reset()
	cmd.SetOutput(buf)
	cmd.SetArgs(args)
	wirek8s.Flags(cmd.PersistentFlags())

	assert.NoError(t, cmd.Execute())
	assert.Equal(t, "Doing `cli-experimental delete`\nResources: 2\n", buf.String())
}
