package observer

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObserverReader is the interface provided to the observers to talk to the cluster. Implementations
// of this interface allows different caching strategies, for example by pre-fetching resources using
// LIST calls rather than letting each observer run multiple GET calls against the cluster. This can
// significantly reduce the number of requests.
type ObserverReader interface {
	// Get looks up the resource identifier by the key and the GVK in the provided obj reference. If something
	// goes wrong or the resource doesn't exist, an error is returned.
	Get(ctx context.Context, key client.ObjectKey, obj *unstructured.Unstructured) error
	// ListNamespaceScoped looks up the resources of the GVK given in the list and matches the namespace and
	// selector provided.
	ListNamespaceScoped(ctx context.Context, list *unstructured.UnstructuredList,
		namespace string, selector labels.Selector) error
	// ListClusterScoped looks up the resources of the GVK given in the list and that matches the selector
	// provided.
	ListClusterScoped(ctx context.Context, list *unstructured.UnstructuredList, selector labels.Selector) error
	// Sync is called by the observer before every polling loop, which provides an opportunity for the Reader
	// to sync caches.
	Sync(ctx context.Context) error
}
