// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestProcessPaths(t *testing.T) {
	trueVal := true
	testCases := map[string]struct {
		paths                     []string
		expectedFileNameFlags     genericclioptions.FileNameFlags
		errFromDemandOneDirectory string
	}{
		"empty slice means reading from StdIn": {
			paths: []string{},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"-"},
			},
			errFromDemandOneDirectory: "argument '-' is not but must be a directory",
		},
		"single file in slice means reading from that path": {
			paths: []string{"object.yaml"},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"object.yaml"},
				Recursive: &trueVal,
			},
			errFromDemandOneDirectory: "argument 'object.yaml' is not but must be a directory",
		},
		"single dir in slice": {
			paths: []string{"/tmp"},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"/tmp"},
				Recursive: &trueVal,
			},
		},
		"multiple elements in slice means reading from all files": {
			paths: []string{"rs.yaml", "dep.yaml"},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"rs.yaml", "dep.yaml"},
				Recursive: &trueVal,
			},
			errFromDemandOneDirectory: "specify exactly one directory path argument; rejecting [rs.yaml dep.yaml]",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			var err error
			fileNameFlags := processPaths(tc.paths)
			assert.DeepEqual(t, tc.expectedFileNameFlags, fileNameFlags)
			fileNameFlags, err = demandOneDirectory(tc.paths)
			assert.DeepEqual(t, tc.expectedFileNameFlags, fileNameFlags)
			if err != nil && err.Error() != tc.errFromDemandOneDirectory {
				assert.Equal(t, err.Error(), tc.errFromDemandOneDirectory)
			}
		})
	}
}
