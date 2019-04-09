# Dynamic Commands

Dynamic Commands are server-side defined Commands published to the client as CRD annotations containing
`ResourceCommandList` objects.

## Sample

### List the Commands from the cli

No Dynamic Commands will be present.

```bash
cli-experimental -h
```

### Create the CRD that publishes Commands

Create a CRD that publishes Dynamic Commands.

```bash
kubectl apply -f .sample/cli_v1alpha1_clitestresource.yaml
```

### List the Commands from the cli

New `cli-experimental create deployment` Command will now appear in help

```bash
cli-experimental -h
```

```bash
cli-experimental create -h
```

```bash
cli-experimental deployment -h
```

### Run the Command in dry-run

Run the command, but print the Resources rather than creating them.

```bash
cli-experimental create deployment --image ubuntu --name foo --dry-run
```

### Run the Command

Run the command to create the Resources.

```bash
cli-experimental create deployment --image ubuntu --name foo
```

## Publishing a Command

### Define the ResourceCommandList in a yaml file

See the [dynamic.v1alpha1](../../internal/pkg/apis/dynamic/v1alpha1/types.go) API for documentation.

### Add the ResourceCommandList to a CRD

Build the `dy` command

```bash
go build ./util/dy
```

```bash
dy add-commands path/to/commands.yaml path/to/crd.yaml
```

### Apply the CRD

```bash
kubectl apply -f path/to/crd.yaml
```

### Run the new Command

```bash
cli-experimental your command -h
```
