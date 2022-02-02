// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Manager wraps an Inventory with convenience methods that use ObjMetadata.
type Manager struct {
	inventory *Inventory
}

// NewManager returns a new manager instance.
func NewManager() *Manager {
	return &Manager{
		inventory: &Inventory{},
	}
}

// Inventory returns the in-memory version of the managed inventory.
func (tc *Manager) Inventory() *Inventory {
	return tc.inventory
}

// ObjectStatus retrieves the status of an object with the specified ID.
// The returned status is a pointer and can be updated in-place for efficiency.
func (tc *Manager) ObjectStatus(id object.ObjMetadata) (*ObjectStatus, bool) {
	for i, objStatus := range tc.inventory.Status.Objects {
		if ObjMetadataEqualObjectReference(id, objStatus.ObjectReference) {
			return &(tc.inventory.Status.Objects[i]), true
		}
	}
	return nil, false
}

// ObjectsWithActuationStatus retrieves the set of objects with the
// specified actuation strategy and status.
func (tc *Manager) ObjectsWithActuationStatus(strategy ActuationStrategy, status ActuationStatus) object.ObjMetadataSet {
	var ids object.ObjMetadataSet
	for _, objStatus := range tc.inventory.Status.Objects {
		if objStatus.Strategy == strategy && objStatus.Actuation == status {
			ids = append(ids, ObjMetadataFromObjectReference(objStatus.ObjectReference))
		}
	}
	return ids
}

// ObjectsWithActuationStatus retrieves the set of objects with the
// specified reconcile status, regardless of actuation strategy.
func (tc *Manager) ObjectsWithReconcileStatus(status ReconcileStatus) object.ObjMetadataSet {
	var ids object.ObjMetadataSet
	for _, objStatus := range tc.inventory.Status.Objects {
		if objStatus.Reconcile == status {
			ids = append(ids, ObjMetadataFromObjectReference(objStatus.ObjectReference))
		}
	}
	return ids
}

// SetObjectStatus updates or adds an ObjectStatus record to the inventory.
func (tc *Manager) SetObjectStatus(id object.ObjMetadata, objStatus ObjectStatus) {
	for i, objStatus := range tc.inventory.Status.Objects {
		if ObjMetadataEqualObjectReference(id, objStatus.ObjectReference) {
			tc.inventory.Status.Objects[i] = objStatus
			return
		}
	}
	tc.inventory.Status.Objects = append(tc.inventory.Status.Objects, objStatus)
}

// IsSuccessfulApply returns true if the object apply was successful
func (tc *Manager) IsSuccessfulApply(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyApply &&
		objStatus.Actuation == ActuationSucceeded
}

// AddSuccessfulApply updates the context with information about the
// resource identified by the provided id. Currently, we keep information
// about the generation of the resource after the apply operation completed.
func (tc *Manager) AddSuccessfulApply(id object.ObjMetadata, uid types.UID, gen int64) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyApply,
		Actuation:       ActuationSucceeded,
		Reconcile:       ReconcilePending,
		UID:             uid,
		Generation:      gen,
	})
}

// SuccessfulApplies returns all the objects (as ObjMetadata) that
// were added as applied resources to the Manager.
func (tc *Manager) SuccessfulApplies() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyApply,
		ActuationSucceeded)
}

// AppliedResourceUID looks up the UID of a successfully applied resource
func (tc *Manager) AppliedResourceUID(id object.ObjMetadata) (types.UID, bool) {
	objStatus, found := tc.ObjectStatus(id)
	return objStatus.UID, found &&
		objStatus.Strategy == ActuationStrategyApply &&
		objStatus.Actuation == ActuationSucceeded
}

// AppliedResourceUIDs returns a set with the UIDs of all the
// successfully applied resources.
func (tc *Manager) AppliedResourceUIDs() sets.String {
	uids := sets.NewString()
	for _, objStatus := range tc.inventory.Status.Objects {
		if objStatus.Strategy == ActuationStrategyApply &&
			objStatus.Actuation == ActuationSucceeded {
			if objStatus.UID != "" {
				uids.Insert(string(objStatus.UID))
			}
		}
	}
	return uids
}

// AppliedGeneration looks up the generation of the given resource
// after it was applied.
func (tc *Manager) AppliedGeneration(id object.ObjMetadata) (int64, bool) {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return 0, false
	}
	return objStatus.Generation, true
}

// IsSuccessfulDelete returns true if the object delete was successful
func (tc *Manager) IsSuccessfulDelete(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyDelete &&
		objStatus.Actuation == ActuationSucceeded
}

// AddSuccessfulDelete updates the context with information about the
// resource identified by the provided id. Currently, we only track the uid,
// because the DELETE call doesn't always return the generation, namely if the
// object was scheduled to be deleted asynchronously, which might cause further
// updates by finalizers. The UID will change if the object is re-created.
func (tc *Manager) AddSuccessfulDelete(id object.ObjMetadata, uid types.UID) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyDelete,
		Actuation:       ActuationSucceeded,
		Reconcile:       ReconcilePending,
		UID:             uid,
	})
}

// SuccessfulDeletes returns all the objects (as ObjMetadata) that
// were successfully deleted.
func (tc *Manager) SuccessfulDeletes() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyDelete,
		ActuationSucceeded)
}

// IsFailedApply returns true if the object failed to apply
func (tc *Manager) IsFailedApply(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyApply &&
		objStatus.Actuation == ActuationFailed
}

// AddFailedApply registers that the object failed to apply
func (tc *Manager) AddFailedApply(id object.ObjMetadata) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyApply,
		Actuation:       ActuationFailed,
		Reconcile:       ReconcilePending,
	})
}

// FailedApplies returns all the objects that failed to apply
func (tc *Manager) FailedApplies() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyApply, ActuationFailed)
}

// IsFailedDelete returns true if the object failed to delete
func (tc *Manager) IsFailedDelete(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyDelete &&
		objStatus.Actuation == ActuationFailed
}

// AddFailedDelete registers that the object failed to delete
func (tc *Manager) AddFailedDelete(id object.ObjMetadata) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyDelete,
		Actuation:       ActuationFailed,
		Reconcile:       ReconcilePending,
	})
}

// FailedDeletes returns all the objects that failed to delete
func (tc *Manager) FailedDeletes() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyDelete, ActuationFailed)
}

// IsSkippedApply returns true if the object apply was skipped
func (tc *Manager) IsSkippedApply(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyApply &&
		objStatus.Actuation == ActuationSkipped
}

// AddSkippedApply registers that the object apply was skipped
func (tc *Manager) AddSkippedApply(id object.ObjMetadata) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyApply,
		Actuation:       ActuationSkipped,
		Reconcile:       ReconcilePending,
	})
}

// SkippedApplies returns all the objects where apply was skipped
func (tc *Manager) SkippedApplies() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyApply, ActuationSkipped)
}

// IsSkippedDelete returns true if the object delete was skipped
func (tc *Manager) IsSkippedDelete(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Strategy == ActuationStrategyDelete &&
		objStatus.Actuation == ActuationSkipped
}

// AddSkippedDelete registers that the object delete was skipped
func (tc *Manager) AddSkippedDelete(id object.ObjMetadata) {
	tc.SetObjectStatus(id, ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        ActuationStrategyDelete,
		Actuation:       ActuationSkipped,
		Reconcile:       ReconcilePending,
	})
}

// SkippedDeletes returns all the objects where deletion was skipped
func (tc *Manager) SkippedDeletes() object.ObjMetadataSet {
	return tc.ObjectsWithActuationStatus(ActuationStrategyDelete, ActuationSkipped)
}

// IsSuccessfulReconcile returns true if the object is reconciled
func (tc *Manager) IsSuccessfulReconcile(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Reconcile == ReconcileSucceeded
}

// SetSuccessfulReconcile registers that the object is reconciled
func (tc *Manager) SetSuccessfulReconcile(id object.ObjMetadata) error {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return fmt.Errorf("object not in inventory: %q", id)
	}
	objStatus.Reconcile = ReconcileSucceeded
	return nil
}

// SuccessfulReconciles returns all the reconciled objects
func (tc *Manager) SuccessfulReconciles() object.ObjMetadataSet {
	return tc.ObjectsWithReconcileStatus(ReconcileSucceeded)
}

// IsFailedReconcile returns true if the object failed to reconcile
func (tc *Manager) IsFailedReconcile(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Reconcile == ReconcileFailed
}

// SetFailedReconcile registers that the object failed to reconcile
func (tc *Manager) SetFailedReconcile(id object.ObjMetadata) error {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return fmt.Errorf("object not in inventory: %q", id)
	}
	objStatus.Reconcile = ReconcileFailed
	return nil
}

// FailedReconciles returns all the objects that failed to reconcile
func (tc *Manager) FailedReconciles() object.ObjMetadataSet {
	return tc.ObjectsWithReconcileStatus(ReconcileFailed)
}

// IsSkippedReconcile returns true if the object reconcile was skipped
func (tc *Manager) IsSkippedReconcile(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Reconcile == ReconcileSkipped
}

// SetSkippedReconcile registers that the object reconcile was skipped
func (tc *Manager) SetSkippedReconcile(id object.ObjMetadata) error {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return fmt.Errorf("object not in inventory: %q", id)
	}
	objStatus.Reconcile = ReconcileSkipped
	return nil
}

// SkippedReconciles returns all the objects where reconcile was skipped
func (tc *Manager) SkippedReconciles() object.ObjMetadataSet {
	return tc.ObjectsWithReconcileStatus(ReconcileSkipped)
}

// IsTimeoutReconcile returns true if the object reconcile was skipped
func (tc *Manager) IsTimeoutReconcile(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Reconcile == ReconcileTimeout
}

// SetTimeoutReconcile registers that the object reconcile was skipped
func (tc *Manager) SetTimeoutReconcile(id object.ObjMetadata) error {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return fmt.Errorf("object not in inventory: %q", id)
	}
	objStatus.Reconcile = ReconcileTimeout
	return nil
}

// TimeoutReconciles returns all the objects where reconcile was skipped
func (tc *Manager) TimeoutReconciles() object.ObjMetadataSet {
	return tc.ObjectsWithReconcileStatus(ReconcileTimeout)
}

// IsPendingReconcile returns true if the object reconcile is pending
func (tc *Manager) IsPendingReconcile(id object.ObjMetadata) bool {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return false
	}
	return objStatus.Reconcile == ReconcilePending
}

// SetPendingReconcile registers that the object reconcile is pending
func (tc *Manager) SetPendingReconcile(id object.ObjMetadata) error {
	objStatus, found := tc.ObjectStatus(id)
	if !found {
		return fmt.Errorf("object not in inventory: %q", id)
	}
	objStatus.Reconcile = ReconcilePending
	return nil
}

// PendingReconciles returns all the objects where reconcile is pending
func (tc *Manager) PendingReconciles() object.ObjMetadataSet {
	return tc.ObjectsWithReconcileStatus(ReconcilePending)
}
