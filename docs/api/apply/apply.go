package apply

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/docs/api/event"
	"sigs.k8s.io/cli-utils/docs/api/inventory"
	"sigs.k8s.io/cli-utils/docs/api/provider"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// NewApplier creates a new applier based on the given provider.
func NewApplier(provider provider.Provider) *applier {
	return nil
}

// applier can apply resources to a cluster. Each instance of the applier
// can be used with a single cluster and using a single strategy for how
// objects for the inventory should be created.
type applier struct {

}

// Options defines the different properties that can be set to adjust
// the behavior of a call to Apply.
type Options struct {
	// ReconcileTimeout defines whether the applier should wait
	// until all applied resources have been reconciled, and if so,
	// how long to wait.
	ReconcileTimeout time.Duration

	// PollInterval defines how often we should poll for the status
	// of resources.
	PollInterval time.Duration

	// EmitStatusEvents defines whether status events should be
	// emitted on the eventChannel to the caller.
	EmitStatusEvents bool

	// NoPrune defines whether pruning of previously applied
	// objects should happen after apply.
	NoPrune bool

	// DryRunStrategy defines whether changes should actually be performed,
	// or if it is just talk and no action.
	DryRunStrategy common.DryRunStrategy

	// PrunePropagationPolicy defines the deletion propagation policy
	// that should be used for pruning. If this is not provided, the
	// default is to use the Background policy.
	PrunePropagationPolicy metav1.DeletionPropagation

	// PruneTimeout defines whether we should wait for all resources
	// to be fully deleted after pruning, and if so, how long we should
	// wait.
	PruneTimeout time.Duration
}

// Apply will apply the provided set of resources to the cluster given by
// the provider used when the applier was created. The inventoryInfo parameter
// contains the data needed to create a new resource for the inventory or to
// look up an existing one. The resource type for the inventory is given by
// the provider used when creating the applier. The options parameter contains
// the properties that can be set to adjust the behavior of Apply, for example
// to make a dry run.
// The return value is a go channel that will provide updates on the progress
// of applying (and optionally pruning) resources. When the operation is
// complete, the channel will be closed.
func (a *applier) Apply(ctx context.Context, inventoryInfo inventory.InventoryInfo,
	resources []*unstructured.Unstructured, options Options) <-chan event.Event {
	return nil
}
