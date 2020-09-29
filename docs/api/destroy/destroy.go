package destroy

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-utils/docs/api/event"
	"sigs.k8s.io/cli-utils/docs/api/inventory"
	"sigs.k8s.io/cli-utils/docs/api/provider"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// NewDestroyer creates a new destroyer based on the given provider
func NewDestroyer(provider provider.Provider) *destroyer {
	return nil
}

// destroyer can delete resources from a cluster based on identifying
// information about the inventory. The cluster that should be targeted
// and the strategy for the inventory object are provided during creation.
type destroyer struct {

}

// Options defines the different properties that can be set to adjust
// the behavior of a call to Destroy.
type Options struct {
	// DryRunStrategy defines whether changes should actually be performed,
	// or if it is just talk and no action.
	DryRunStrategy common.DryRunStrategy

	// DeletePropagationPolicy defines the deletion propagation policy
	// that should be used when the destroyer deletes resources. If this is
	// not provided, the default is to use the Background policy.
	DeletePropagationPolicy metav1.DeletionPropagation

	// DeleteTimeout defines whether we should wait for all resources
	// to be fully deleted after pruning, and if so, how long we should
	// wait.
	DeleteTimeout time.Duration
}

// Destroy will delete all resources from the cluster listed in the inventory
// of the resource identified in the inventoryInfo. The options parameter
// contains the properties that can be set to adjust the behavior of Destroy,
// for example to make a dry run.
// The return value is a go channel that will provide updates on the progress
// of destroying the resources. When the operation is complete, the channel
// will be closed.
func (d *destroyer) Destroy(ctx context.Context, inventoryInfo inventory.InventoryInfo,
	options Options) <-chan event.Event {
	return nil
}
