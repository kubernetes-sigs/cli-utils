package apply

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestProcessPaths(t *testing.T) {
	trueVal := true
	testCases := map[string]struct {
		paths                 []string
		expectedFileNameFlags genericclioptions.FileNameFlags
	}{
		"empty slice means reading from StdIn": {
			paths: []string{},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"-"},
			},
		},
		"single element in slice means reading from that file/path": {
			paths: []string{"object.yaml"},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"object.yaml"},
				Recursive: &trueVal,
			},
		},
		"multiple elements in slice means reading from all files": {
			paths: []string{"rs.yaml", "dep.yaml"},
			expectedFileNameFlags: genericclioptions.FileNameFlags{
				Filenames: &[]string{"rs.yaml", "dep.yaml"},
				Recursive: &trueVal,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fileNameFlags := processPaths(tc.paths)

			assert.DeepEqual(t, tc.expectedFileNameFlags, fileNameFlags)
		})
	}
}
