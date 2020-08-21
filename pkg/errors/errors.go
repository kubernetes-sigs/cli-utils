// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

const (
	DefaultErrorExitCode = 1
	TimeoutErrorExitCode = 3
)

var errorMsgForType map[reflect.Type]string
var statusCodeForType map[reflect.Type]int

//nolint:gochecknoinits
func init() {
	errorMsgForType = make(map[reflect.Type]string)
	errorMsgForType[reflect.TypeOf(inventory.NoInventoryObjError{})] = `
Package uninitialized. Please run "{{.cmdNameBase}} init" command.

The package needs to be initialized to generate the template
which will store state for resource sets. This state is
necessary to perform functionality such as deleting an entire
package or automatically deleting omitted resources (pruning).
`

	errorMsgForType[reflect.TypeOf(inventory.MultipleInventoryObjError{})] = `
Package has multiple inventory object templates.

The package should have one and only one inventory object template.
`
	//nolint:lll
	errorMsgForType[reflect.TypeOf(taskrunner.TimeoutError{})] = `
Timeout after {{printf "%.0f" .err.Timeout.Seconds}} seconds waiting for {{printf "%d" (len .err.TimedOutResources)}} out of {{printf "%d" (len .err.Identifiers)}} resources to reach condition {{ .err.Condition}}:

{{- range .err.TimedOutResources}}
{{printf "%s/%s %s %s" .Identifier.GroupKind.Kind .Identifier.Name .Status .Message }}
{{- end}}
`

	statusCodeForType = make(map[reflect.Type]int)
	statusCodeForType[reflect.TypeOf(taskrunner.TimeoutError{})] = TimeoutErrorExitCode
}

// CheckErr looks up the appropriate error message and exit status for known
// errors. It will print the information to the provided io.Writer. If we
// don't know the error, it delegates to the error handling in cmdutil.
func CheckErr(w io.Writer, err error, cmdNameBase string) {
	errText, found := textForError(err, cmdNameBase)
	if found {
		exitStatus := findErrExitCode(err)
		if len(errText) > 0 {
			if !strings.HasSuffix(errText, "\n") {
				errText += "\n"
			}
			fmt.Fprint(w, errText)
		}
		os.Exit(exitStatus)
	}

	cmdutil.CheckErr(err)
}

// textForError looks up the error message based on the type of the error.
func textForError(baseErr error, cmdNameBase string) (string, bool) {
	errType, found := findErrType(baseErr)
	if !found {
		return "", false
	}
	tmplText, found := errorMsgForType[errType]
	if !found {
		return "", false
	}

	tmpl, err := template.New("errMsg").Parse(tmplText)
	if err != nil {
		// Just return false here instead of the error. It will just
		// mean a less informative error message and we rather show the
		// original error.
		return "", false
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, map[string]interface{}{
		"cmdNameBase": cmdNameBase,
		"err":         baseErr,
	})
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(b.String()), true
}

// findErrType finds the type of the error. It returns the real type in the
// event the error is actually a pointer to a type.
func findErrType(err error) (reflect.Type, bool) {
	switch reflect.ValueOf(err).Kind() {
	case reflect.Ptr:
		// If the value of the interface is a pointer, we use the type
		// of the real value.
		return reflect.ValueOf(err).Elem().Type(), true
	case reflect.Struct:
		return reflect.TypeOf(err), true
	default:
		return nil, false
	}
}

// findErrExitCode looks up if there is a defined error code for the provided
// error type.
func findErrExitCode(err error) int {
	errType, found := findErrType(err)
	if !found {
		return DefaultErrorExitCode
	}
	if exitStatus, found := statusCodeForType[errType]; found {
		return exitStatus
	}
	return DefaultErrorExitCode
}
