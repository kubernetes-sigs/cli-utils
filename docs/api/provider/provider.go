package provider

import (
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
)

// Provider is the interface used when creating an Applier or a Destroyer. It
// encapsulates information about which cluster the operations should be
// targeting, and the logic needed to create and update inventory objects
// of a specific resource type.
type Provider interface {
	Factory() util.Factory
	InventoryClient() (inventory.InventoryClient, error)
	ToRESTMapper() (meta.RESTMapper, error)
	ManifestReader(reader io.Reader, args []string) (manifestreader.ManifestReader, error)
}
