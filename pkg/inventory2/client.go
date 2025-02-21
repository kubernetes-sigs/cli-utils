package inventory2

import (
	"context"

	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ID client.ObjectKey

type Client interface {
	ReadClient
	WriteClient
}

type ReadClient interface {
	Get(ctx context.Context, id ID, opts ...GetOption) (*actuation.Inventory, error)
	List(ctx context.Context, opts ...ListOption) error
}

type WriteClient interface {
	Create(ctx context.Context, inv *actuation.Inventory, opts ...CreateOption) error
	Update(ctx context.Context, inv *actuation.Inventory, opts ...UpdateOption) error
	Delete(ctx context.Context, inv *actuation.Inventory, opts ...DeleteOption) error
}

type CreateOption interface {
	ApplyCreateOptions(opts *CreateOptions)
}

type CreateOptions struct {
	DryRunStrategy common.DryRunStrategy
	StatusPolicy   inventory.StatusPolicy
}

type GetOption interface {
	ApplyGetOptions(opts *GetOptions)
}

type GetOptions struct {
	ResourceVersion string
	LabelSelector   string
}

type UpdateOption interface {
	ApplyUpdateOptions(opts *UpdateOptions)
}

type UpdateOptions struct {
	DryRunStrategy common.DryRunStrategy
	StatusPolicy   inventory.StatusPolicy
}

type DeleteOption interface {
	ApplyDeleteOptions(opts *DeleteOptions)
}

type DeleteOptions struct {
	DryRunStrategy common.DryRunStrategy
}

type ListOption interface {
	ApplyListOptions(opts *ListOptions)
}

type ListOptions struct {
	ResourceVersion string
	LabelSelector   string
}

func WithDryRun(strategy common.DryRunStrategy) DryRunOption {
	return DryRunOption(strategy)
}

type DryRunOption common.DryRunStrategy

func (o DryRunOption) ApplyCreateOptions(opts *CreateOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

func (o DryRunOption) ApplyUpdateOptions(opts *UpdateOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

func (o DryRunOption) ApplyDeleteOptions(opts *DeleteOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

var _ CreateOption = DryRunOption(common.DryRunServer)
var _ UpdateOption = DryRunOption(common.DryRunServer)
var _ DeleteOption = DryRunOption(common.DryRunServer)

func WithStatus(policy inventory.StatusPolicy) StatusOption {
	return StatusOption(policy)
}

type StatusOption common.DryRunStrategy

func (o StatusOption) ApplyCreateOptions(opts *CreateOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

func (o StatusOption) ApplyUpdateOptions(opts *UpdateOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

func (o StatusOption) ApplyDeleteOptions(opts *DeleteOptions) {
	opts.DryRunStrategy = common.DryRunStrategy(o)
}

var _ CreateOption = StatusOption(inventory.StatusPolicyAll)
var _ UpdateOption = StatusOption(inventory.StatusPolicyAll)
var _ DeleteOption = StatusOption(inventory.StatusPolicyAll)
