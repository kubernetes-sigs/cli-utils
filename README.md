# cli-utils

The cli-utils repository contains an actuation library, which wraps `kubectl apply` code.
This library allows importers to easily execute `kubectl apply`, while also
addressing several deficiencies of the current implementation of `kubectl apply`.
The library enhances `kubectl apply` in the following ways:

1. **Pruning**: adds new, experimental automatic object deletion functionality.
2. **Sorting**: adds optional resource sorting functionality to apply or delete objects
in a particular order.
3. **Apply Time Mutation**: adds optional functionality to dynamically substitute fields
from one resource config into another.

TODO(seans): Add examples of API, once we've achieved an alpha API.

### Pruning

The current implementation of `kubectl apply --prune` uses labels to identify the
set of previously applied objects in the prune set calculation. But the use of labels
has significant downsides. The current `kubectl apply --prune` implemenation is alpha,
and it is improbable that it will graduate to beta. This library attempts to address
the current `kubectl apply --prune` deficiencies by storing the set of previously
applied objects in an **inventory** object which is applied to the cluster. The
**inventory** object is a `ConfigMap` with the `inventory-id` label, and references
to the applied objects are stored in the `data` section of the `ConfigMap`.

The following example illustrates a `ConfigMap` resource used as an inventory object:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  # DANGER: Do not change the inventory object namespace.
  # Changing the namespace will cause a loss of continuity
  # with previously applied grouped objects. Set deletion
  # and pruning functionality will be impaired.
  namespace: test-namespace
  # NOTE: The name of the inventory object does NOT have
  # any impact on group-related functionality such as
  # deletion or pruning.
  name: inventory-26306433
  labels:
    # DANGER: Do not change the value of this label.
    # Changing this value will cause a loss of continuity
    # with previously applied grouped objects. Set deletion
    # and pruning functionality will be impaired.
    cli-utils.sigs.k8s.io/inventory-id: 46d8946c-c1fa-4e1d-9357-b37fb9bae25f
```

### Apply Sort Ordering

Adding an optional `config.kubernetes.io/depends-on: <OBJECT>` annotation to a
resource config provides apply ordering functionality. After manually specifying
the dependency relationship among applied resources with this annotation, the
library will sort the resources and apply/prune them in the correct order.
Importantly, the library will wait for an object to reconcile successfully within
the cluster before applying dependent resources. Prune (deletion) ordering is
the opposite of apply ordering.

In the following example, the `config.kubernetes.io/depends-on` annotation
identifies that `pod-c` must be successfully applied prior to `pod-a`
actuation:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-a
  annotations:
    config.kubernetes.io/depends-on: /namespaces/default/Pod/pod-c
spec:
  containers:
    - name: kubernetes-pause
      image: k8s.gcr.io/pause:2.0
```

### Apply-Time Mutation

**apply-time mutation** functionality allows library users to dynamically fill in
resource field values from one object into another, even though they are applied
at the same time. By adding a `config.kubernetes.io/apply-time-mutation` annotation,
a resource specifies the field in another object as well as the location for the
local field subsitution. For example, if an object's IP address is set during
actuation, another object applied at the same time can reference that IP address.
This functionality leverages the previously described **Apply Sort Ordering** to
ensure the source resource field is populated before applying the target resource.

In the following example, `pod-a` will substitute the IP address/port from the
source `pod-b` into the `pod-a` SERVICE_HOST environment variable:

```yaml
kind: Pod
apiVersion: v1
metadata:
  name: pod-a
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: Pod
          name: pod-b
        sourcePath: $.status.podIP
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-ip}
      - sourceRef:
          kind: Pod
          name: pod-b
        sourcePath: $.spec.containers[?(@.name=="nginx")].ports[?(@.name=="tcp")].containerPort
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-port}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
    env:
    - name: SERVICE_HOST
      value: "${pob-b-ip}:${pob-b-port}"
```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-cli)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cli)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
