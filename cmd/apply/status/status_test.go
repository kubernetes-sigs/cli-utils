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

package status_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-experimental/cmd/apply/status"
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

func setupKustomize(t *testing.T) string {
	f, err := ioutil.TempDir("/tmp", "TestApplyStatus")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: testMap
`), 0644)
	assert.NoError(t, err)
	return f
}

func TestStatus(t *testing.T) {
	buf := new(bytes.Buffer)

	args := []string{fmt.Sprintf("--server=%s", host), "--namespace=default", setupKustomize(t)}
	cmd := status.GetApplyStatusCommand(args)
	cmd.SetOutput(buf)
	cmd.SetArgs(args)
	wirek8s.Flags(cmd.PersistentFlags())

	assert.NoError(t, cmd.Execute())
	assert.Equal(t, "Doing `cli-experimental apply status`\nResources: 1\n", buf.String())
}
