// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"

	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// StatusWatcher watches a set of objects for status updates.
type StatusWatcher interface {

	// Watch a set of objects for status updates.
	// Watching should stop if the context is cancelled.
	// Events should only be sent for the specified objects.
	// The event channel should be closed when the watching stops.
	Watch(context.Context, object.ObjMetadataSet, Options) <-chan event.Event
}

// Options can be provided when creating a new StatusWatcher to customize the
// behavior.
type Options struct {
	// RESTScopeStrategy specifies which strategy to use when listing and
	// watching resources. By default, the strategy is selected automatically.
	RESTScopeStrategy RESTScopeStrategy

	// ObjectFilter is used to filter objects on the client side.
	ObjectFilter ObjectFilter

	// UseCustomObjectFilter controls whether custom ObjectFilter provided in options
	// should be used instead of the default one.
	UseCustomObjectFilter bool
}

//go:generate stringer -type=RESTScopeStrategy -linecomment
type RESTScopeStrategy int

const (
	RESTScopeAutomatic RESTScopeStrategy = iota // automatic
	RESTScopeRoot                               // root
	RESTScopeNamespace                          // namespace
)
