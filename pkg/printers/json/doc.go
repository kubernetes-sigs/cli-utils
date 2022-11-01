// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package json provides a printer that outputs the eventstream in json
// format. Each event is printed as a json object, so the output will
// appear as a stream of json objects, each representing a single event.
//
// Every event will contain the following properties:
//   - timestamp: RFC3339-formatted timestamp describing when the event happened.
//   - type: Describes the type of the operation which the event is related to.
//     Type values include:
//   - validation - ValidationEvent
//   - error - ErrorEvent
//   - group - ActionGroupEvent
//   - apply - ApplyEvent
//   - prune - PruneEvent
//   - delete - DeleteEvent
//   - wait - WaitEvent
//   - status - StatusEvent
//   - summary - aggregate stats collected by the printer
//
// Validation events correspond to zero or more objects. For these events, the
// objects field includes a list of object identifiers. These generally fire
// first before most other events.
//
// Validation events have the following fields:
// * objects (array of objects) - a list of object identifiers
//   - group (string, optional) - The object's API group.
//   - kind (string) - The object's kind.
//   - name (string) - The object's name.
//   - namespace (string, optional) - The object's namespace.
//
// * timestamp (string) - ISO-8601 format
// * type (string) - "validation"
// * error (string) - a fatal error message specific to these objects
//
// Error events corespond to a fatal error received outside of a specific task
// or operation.
//
// Error events have the following fields:
// * timestamp (string) - ISO-8601 format
// * type (string) - "error"
// * error (string)  - a fatal error message
//
// Group events correspond to a group of events of the same type: apply, prune,
// delete, or wait.
//
// Group events have the following fields:
// * action (string) - One of: "Apply", "Prune", "Delete", or "Wait".
// * status (string) - One of: "Started" or "Finished"
// * timestamp (string) - ISO-8601 format
// * type (string) - "group"
//
// Operation events (apply, prune, delete, and wait) corespond to an operation
// performed on a single object. For these events, the
// group, kind, name, and namespace fields identify the object.
//
// Operation events have the following fields:
//   - group (string, optional) - The object's API group.
//   - kind (string) - The object's kind.
//   - name (string) - The object's name.
//   - namespace (string, optional) - The object's namespace.
//   - status (string) - One of: "Pending", "Successful", "Skipped", "Failed", or
//     "Timeout".
//   - timestamp (string) - ISO-8601 format
//   - type (string) - "apply", "prune", "delete", or "wait"
//   - error (string, optional) - A non-fatal error message specific to this object
//
// Status types are asynchronous events that correspond to status updates for
// a specific object.
//
// Status events have the following fields:
//   - group (string, optional) - The object's API group.
//   - kind (string) - The object's kind.
//   - name (string) - The object's name.
//   - namespace (string, optional) - The object's namespace.
//   - status (string) - One of: "InProgress", "Failed", "Current", "Terminating",
//     "NotFound", or "Unknown".
//   - message (string) - Human readable description of the status.
//   - timestamp (string) - ISO-8601 format
//   - type (string) - "status"
//
// Summary types are a meta-event sent by the printer to summarize some stats
// that have been collected from other events. For these events, the action
// field corresponds to the event type being summarized: Apply, Prune, Delete,
// and Wait.
//
// Summary events have the following fields:
// * action (string) - One of: "Apply", "Prune", "Delete", or "Wait".
// * count (number) - Total number of objects attempted for this action
// * successful (number) - Number of objects for which the action was successful.
// * skipped (number) - Number of objects for which the action was skipped.
// * failed (number) - Number of objects for which the action failed.
// * timeout (number, optional) - Number of objects for which the action timed out.
// * timestamp (string) - ISO-8601 format
// * type (string) - "summary"
package json
