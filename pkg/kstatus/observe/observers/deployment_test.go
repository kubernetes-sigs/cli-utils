package observers

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/testutil"
)

func TestDeploymentObserver(t *testing.T) {
	deploymentManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
`

	replicaSetManifest1 := `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: Foo-12345
spec:
  replicas: 1
`

	replicaSetManifest2 := `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: Foo-54321
spec:
  replicas: 14
`

	replicaSetGVK := appsv1.SchemeGroupVersion.WithKind("ReplicaSet")

	generatedObjects := []unstructured.Unstructured{
		*testutil.YamlToUnstructured(t, replicaSetManifest1),
		*testutil.YamlToUnstructured(t, replicaSetManifest2),
	}

	var newRsObserverFunc newResourceObserverFunc = func(reader observer.ClusterReader, mapper meta.RESTMapper) observer.ResourceObserver {
		return NewDeploymentObserver(reader, mapper, &fakeResourceObserver{})
	}

	basicObserverTest(t, deploymentManifest, replicaSetGVK, generatedObjects, newRsObserverFunc)
}
