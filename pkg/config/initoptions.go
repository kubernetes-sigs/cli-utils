// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

const manifestFilename = "grouping-object-template.yaml"

const configMapTemplate = `# NOTE: auto-generated. Some fields should NOT be modified.
# Date: <DATETIME>
#
# Contains the "grouping object" template ConfigMap.
# When this object is applied, it is handled specially,
# storing the metadata of all the other objects applied.
# This object and its stored inventory is subsequently
# used to calculate the set of objects to automatically
# delete (prune), when an object is omitted from further
# applies. When applied, this "grouping object" is also
# used to identify the entire set of objects to delete.
#
# NOTE: The name of this grouping object template file
# (e.g. ` + manifestFilename + `) does NOT have any
# impact on group-related functionality such as deletion
# or pruning.
#
apiVersion: v1
kind: ConfigMap
metadata:
  # DANGER: Do not change the grouping object namespace.
  # Changing the namespace will cause a loss of continuity
  # with previously applied grouped objects. Set deletion
  # and pruning functionality will be impaired.
  namespace: <NAMESPACE>
  # NOTE: The name of the grouping object does NOT have
  # any impact on group-related functionality such as
  # deletion or pruning.
  name: grouping-object
  labels:
    # DANGER: Do not change the value of this label.
    # Changing this value will cause a loss of continuity
    # with previously applied grouped objects. Set deletion
    # and pruning functionality will be impaired.
    cli-utils.sigs.k8s.io/inventory-id: <GROUPNAME>
`

// InitOptions contains the fields necessary to generate a
// grouping object template ConfigMap.
type InitOptions struct {
	ioStreams genericclioptions.IOStreams
	// Package directory argument
	Dir string
	// Namespace for grouping object
	Namespace string
	// Grouping object label value
	GroupName string
	// Random seed
	Seed int64
}

func NewInitOptions(ioStreams genericclioptions.IOStreams) *InitOptions {
	return &InitOptions{
		ioStreams: ioStreams,
	}
}

// Complete fills in the InitOptions fields.
// TODO(seans3): Look into changing this kubectl-inspired way of organizing
// the InitOptions (e.g. Complete and Run methods).
func (i *InitOptions) Complete(args []string) error {
	i.Seed = time.Now().UnixNano()
	if len(args) != 1 {
		return fmt.Errorf("need one 'directory' arg; have %d", len(args))
	}
	i.Dir = args[0]
	if !isDirectory(i.Dir) {
		return fmt.Errorf("invalid directory argument: %s", i.Dir)
	}
	if len(i.Namespace) == 0 {
		namespace, err := packageNamespace(i.Dir)
		if err != nil {
			return err
		}
		i.Namespace = namespace
	}
	if len(i.GroupName) == 0 {
		i.GroupName = i.defaultGroupName()
	}
	if !validateGroupName(i.GroupName) {
		return fmt.Errorf("invalid group name: %s", i.GroupName)
	}
	return nil
}

// isDirectory returns true if the passed path is a directory;
// false otherwise.
func isDirectory(path string) bool {
	if d, err := os.Stat(path); err == nil {
		if d.IsDir() {
			return true
		}
	}
	return false
}

// packageNamespace returns the namespace of the package
// config files.
func packageNamespace(packageDir string) (string, error) {
	r := kio.LocalPackageReader{PackagePath: packageDir}
	nodes, err := r.Read()
	if err != nil {
		return "", err
	}
	namespace := "default"
	for _, node := range nodes {
		rm, err := node.GetMeta()
		if err != nil {
			continue
		}
		if len(rm.ObjectMeta.Namespace) > 0 {
			namespace = rm.ObjectMeta.Namespace
		}
		break
	}
	return namespace, nil
}

// defaultGroupName returns a string of the package name (directory)
// with a random number suffix.
func (i *InitOptions) defaultGroupName() string {
	rand.Seed(i.Seed)
	r := rand.Intn(1000000)
	return fmt.Sprintf("%s-%06d", filepath.Base(i.Dir), r)
}

const groupNameRegexp = `^[a-zA-Z0-9-_\.]+$`

// validateGroupName returns true of the passed group name is a
// valid label value; false otherwise. The valid label values
// are [a-z0-9A-Z] "-", "_", and "." The groupName must not
// be empty, but it can not be more than 63 characters.
func validateGroupName(groupName string) bool {
	if len(groupName) == 0 || len(groupName) > 63 {
		return false
	}
	re := regexp.MustCompile(groupNameRegexp)
	return re.MatchString(groupName)
}

// fileExists returns true if a file at path already exists;
// false otherwise.
func fileExists(path string) bool {
	f, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !f.IsDir()
}

// fillInValues returns a string of the grouping object template
// ConfigMap with values filled in (eg. namespace, groupname).
// TODO(seans3): Look into text/template package.
func (i *InitOptions) fillInValues() string {
	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05 MST")
	manifestStr := configMapTemplate
	manifestStr = strings.ReplaceAll(manifestStr, "<DATETIME>", nowStr)
	manifestStr = strings.ReplaceAll(manifestStr, "<NAMESPACE>", i.Namespace)
	manifestStr = strings.ReplaceAll(manifestStr, "<GROUPNAME>", i.GroupName)
	return manifestStr
}

func (i *InitOptions) Run() error {
	manifestFilePath := filepath.Join(i.Dir, manifestFilename)
	if fileExists(manifestFilePath) {
		return fmt.Errorf("grouping object template file already exists: %s", manifestFilePath)
	}
	f, err := os.Create(manifestFilePath)
	if err != nil {
		return fmt.Errorf("unable to create grouping object template file: %s", err)
	}
	defer f.Close()
	_, err = f.WriteString(i.fillInValues())
	if err != nil {
		return fmt.Errorf("unable to write grouping object template file: %s", manifestFilePath)
	}
	fmt.Fprintf(i.ioStreams.Out, "Initialized: %s\n", manifestFilePath)
	return nil
}
