# cli-utils

`cli-utils` is a collection of Go libraries designed to facilitate bulk
actuation of Kubernetes resource objects by wraping and enahancing
`kubectl apply` with a more user friendly abstraction.

While the name indicates a focus on CLI utilities, the project has evolved to
encompass a broader scope, including CLI use and server-side use in GitOps
controllers.

## Features

1. **Pruning**
1. **Status Interpretation**
1. **Status Lookup**
1. **Diff & Preview**
1. **Waiting for Reconciliation**
1. **Resource Ordering**
1. **Explicit Dependency Ordering**
1. **Implicit Dependency Ordering**
1. **Apply Time Mutation**
1. **CLI Printers**

### Pruning

The Applier automatically deletes objects that were previously applied and then
removed from the input set on a subsequent apply.

The current implementation of `kubectl apply --prune` uses labels to identify the
set of previously applied objects in the prune set calculation. But the use of labels
has significant downsides. The current `kubectl apply --prune` implementation is alpha,
and it is improbable that it will graduate to beta. `cli-utils` attempts to address
the current `kubectl apply --prune` deficiencies by storing the set of previously
applied objects in an **inventory** object which is applied to the cluster. The
reference implementation uses a `ConfigMap` as an **inventory** object, and references
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

### Status Interpretation

The `kstatus` library can be used to read an object's current status and interpret
whether that object has be reconciled (aka Current) or not, including whether it
is expected to never reconcile (aka Failed).

### Status Lookup

In addition to performing interpretation of status from an object in-memory,
`cli-utils` can also be used to query status from the server, allowing you to
retrieve the status of previously or concurrently applied objects.

### Diff & Preview

`cli-utils` can be used to compare local object manifests with remote objects
from the server. These can be compared locally with diff or remotely with
preview (aka dry-run). This can be useful for discovering drift or previewing
which changes would be made, if the local manifests were applied.

### Waiting for Reconciliation

The Applier automatically watches applied and deleted objects and tracks their
status, blocking until the objects have reconciled, failed, or been fully
deleted.

This functionality is similar to `kubectl delete <resource> <name> --wait`, in
that it waits for all finalizers to complete, except it also works for creates
and updates.

While there is a `kubectl apply <resource> <name> --wait`, it only waits for
deletes when combined with `--prune`. `cli-utils` provides an alternative that
works for all spec changes, waiting for reconciliation, the convergence of
status to the desired specification. After reconciliation, it is expected that
the object has reached a steady state until the specification is changed again.

### Resource Ordering

The Applier and Destroyer use resource type to determine which order to apply
and delete objects.

In contrast, when using `kubectl apply`, the objects are applied in alphanumeric
order of their file names, and top to bottom in each file. With `cli-utils`,
this manual sorting is unnecessary for many common use cases.

### Explicit Dependency Ordering

While resource ordering provides a smart default user experience, sometimes
resource type alone is not enough to determine desired ordering. In these cases,
the user can use explicit dependency ordering by adding a
`config.kubernetes.io/depends-on: <OBJECT_REFERENCE>` annotation to an object.

The Applier and Destroyer use these explicit dependency directives to build a
dependency tree and flatten it for determining apply ordering. When deleting,
the order is reversed, ensuring that dependencies are not deleted before the
objects that depend on them (aka dependents).

In addition to ordering the applies and deletes, dependency ordering also waits
for dependency reconciliation when applying and deletion finalization when
deleting. This ensures that dependencies are not just applied first, but have
reconciled before their dependents are applied. Likewise, dependents are not
just deleted first, but have completed finalization before their dependencies
are deleted.

Also, because dependency ordering is enforced during actuation, a dependency
cannot be pruned by the Applier unless all its dependents are also deleted. This
prevents accidental premature deletion of objects that are still in active use.

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

### Implicit Dependency Ordering

In addition to being able to specify explicit dependencies, `cli-utils`
automatically detects some implicit dependencies.

Implicit dependencies include:

1. Namespace-scoped resource objects depend on their Namespace.
2. Custom resource objects depend on their Custom Resource Definition

Like resource ordering, implicit dependency ordering improves the apply and
delete experience to reduce the need to manually specify ordering for many
common use cases. This allows more objects to be applied together all at once,
with less manual orchestration.

### Apply-Time Mutation

The Applier can dynamically modify objects before applying them, performing
field value substitution using input(s) from dependency fields.

This allows for applying objects together in a set that you would otherwise need
to seperate into multiple sets, with manual modifications between applies.

Apply-Time Mutation is configured using the
`config.kubernetes.io/apply-time-mutation` annotation on the target object to be
modified. The annotation may specify one or more substitutions. Each
substitution includes a source object, and source field path, and a target
field path, with an optional token. 

If the token is specified, the token is
replaced in the target field value string with the source field value. If the
token is not specified, the whole target field value is replaced with the
source field value. This alternatively allows either templated interpretation or
type preservation.

The source and target field paths are specified using JSONPath, allowing for
robust navigation of complex resource field hierarchies using a familiar syntax.

In the following example, `pod-a` will substitute the IP address and port from
the spec and status of the source `pod-b` into the spec of the target `pod-a`:

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

The primary reason to do this with Apply-Time Mutation, instead of client-side
manifest templating is that the pod IP is populated by a controller at runtime
during reconciliation, and is not known before applying. 

That said, this is a toy example using built-in types. For pods, you probably
actually want to use DNS for service discovery instead. 

Most use cases for Apply-Time Mutation are actually using custom resources, as a
temporary alternative to building higher level abstractions, modifying
interfaces, or creating dependencies between otherwise independent interfaces.

### CLI Printers

Since the original intent of `cli-utils` was to contain common code for CLIs,
and end-to-end testing requires a reference implementation, a few printers are
included to translate from the primary event stream into STDOUT text:

1. **Event Printer**: The event printer just prints text to STDOT whenever an
    event is recieved.
1. **JSON Printer**: The JSON printer converts events into a JSON string per
    line, intended for automated interpretation by machine.
1. **Table Printer**: The table  printer writes and updates in-place a table
    with one object per line, intended for human consumption.

## Packages

├── **cmd**: the kapply CLI command
├── **examples**: examples that serve as additional end-to-end tests using mdrip
├── **hack**: hacky scripts used by make
├── **pkg**
│   ├── **apis**: API resources that satisfy the kubernetes Object interface
│   ├── **apply**: bulk applier and destroyer
│   ├── **common**: placeholder for common tools that should probably have their own package
│   ├── **config**: inventory config bootstrapping
│   ├── **errors**: error printing
│   ├── **flowcontrol**: flow control enablement discovery
│   ├── **inventory**: inventory resource reference implementation
│   ├── **jsonpath**: utility for using jsonpath to read & write Unstructured object fields
│   ├── **kstatus**: object status event watcher with ability to reduce status to a single enum
│   ├── **manifestreader**: bolk resource object manifest reading and parsing
│   ├── **multierror**: error composition
│   ├── **object**: library for dealing with Unstructured objects
│   ├── **ordering**: sort functionality for objects
│   ├── **print**: CLI output
│   ├── **printers**: CLI output
│   └── **testutil**: utility for facilitating testing
├── **release**: goreleaser config
├── **scripts**: scripts used by make
└── **test**: end-to-end and stress tests

## kapply

To facilitate testing, this repository includes a reference CLI called `kapply`.
The `kapply` tool is not intended for direct consumer use, but may be useful
when trying to determine how to best utilize the `cli-utils` library packages.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-cli)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cli)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
