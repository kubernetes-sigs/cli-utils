# apply

```
cli-utils apply <dir>
```
works for both directories with or without `kustomization.yaml`.

## kustomization directory

The [hello](../config/hello) example contains a `kustomization.yaml`, you can apply it by
```
cli-utils apply config/hello
```

The output is like the following
```
Doing `cli-utils apply`
applied Pod/myapp-pod
applied ConfigMap/example-cfgmap
applied StatefulSet/web
applied Deployment/frontend
Resources: 4
```

Delete the applied resources by
```
cli-utils delete config/hello
```

## non kustomization directory

The [manifests](../config/manifests) example doesn't contain a `kustomization.yaml`, you can apply it by
```
cli-utils apply config/manifests
```

The output is like the following

```
Doing `cli-utils apply`
applied ConfigMap/example-cfgmap
applied Deployment/frontend
applied Pod/myapp-pod
Resources: 3
```

Delete the applied resources by
```
cli-utils delete config/manifests
```
