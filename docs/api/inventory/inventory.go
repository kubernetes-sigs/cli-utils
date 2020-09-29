package inventory

// InventoryInfo captures the information needed to create, find and
// delete a resource containing the inventory information. The resource
// can be of any type and the provider passed into the Applier/Destroyer
// contains the logic to look up and update the resource using the information
// provided here.
type InventoryInfo struct {

	// Name is the value of the `metadata.name` field of the object
	// containing the inventory information.
	Name string

	// Namespace is the value of the `metadata.namespace` field of the object
	// containing the inventory information.
	Namespace string

	// Id is a generated value that is added to the inventory object as an
	// annotation with the `config.kubernetes.io/inventory-id key. If the value
	// is "", it will be ignored.
	// If this value is provided and there already exists an inventory object
	// with the provided name and namespace in the cluster, the apply/destroy
	// logic will verify that the annotation value found on the object matches
	// the value provided.
	Id string

	// Labels are labels that will be set on the inventory object when it
	// is created or updated. The provided labels will replace any existing
	// labels if there is already an inventory object.
	Labels map[string]string

	// Annotations are annotations that will be set on the inventory object when it
	// is created or updated. The provided annotations will replace any existing
	// annotations if there is already an inventory object.
	Annotations map[string]string
}
