# Contributing Guidelines

Welcome to Kubernetes. We are excited about the prospect of you joining our [community](https://git.k8s.io/community)! The Kubernetes community abides by the CNCF [code of conduct](code-of-conduct.md). Here is an excerpt:

_As contributors and maintainers of this project, and in the interest of fostering an open and welcoming community, we pledge to respect all people who contribute through reporting issues, posting feature requests, updating documentation, submitting pull requests or patches, and other activities._

## Getting Started

We have full documentation on how to get started contributing here:

<!---
If your repo has certain guidelines for contribution, put them here ahead of the general k8s resources
-->

- [Contributor License Agreement](https://git.k8s.io/community/CLA.md) Kubernetes projects require that you sign a Contributor License Agreement (CLA) before we can accept your pull requests
- [Kubernetes Contributor Guide](https://git.k8s.io/community/contributors/guide) - Main contributor documentation, or you can just jump directly to the [contributing section](https://git.k8s.io/community/contributors/guide#contributing)
- [Contributor Cheat Sheet](https://git.k8s.io/community/contributors/guide/contributor-cheatsheet.md) - Common resources for existing developers

## Mentorship

- [Mentoring Initiatives](https://git.k8s.io/community/mentoring) - We have a diverse set of mentorship programs available that are always looking for volunteers!

## Contact Information

- [Slack channel](https://kubernetes.slack.com/messages/sig-cli)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cli)

## Setup a Dev Environment

- install [go](https://golang.org/doc/install)
- `export GO111MODULE=on`
- install [wire](https://github.com/google/wire/)

## Build and Test

1. `go generate`
  - Generates the `wire_gen.go` files
1. `go test ./...`
  - Test the
1. `golint -min_confidence 0.9 ./...`
  - Look for errors
1. `go build`
  - Build the binary

## Dependency Injection

This repo uses Dependency Injection for wiring together the Commands.  See the
[wire tutorial](https://github.com/google/wire/tree/master/_tutorial) for more on DI.

## Adding a Command

1. Add a new package for your cobra command under `cmd/`
  - e.g. `kubectl apply status` would be added under `cmd/apply/status`
  - Add it to the parent command
  - Copy an existing command as an example
1. Add a new package that contains the library for your command under `internal/pkg`
  - e.g. `kubectl apply status` library would be added under `internal/pkg/status`
  - Invoke it from the command you added
  - Copy an existing package as an example
1. Add the DI wiring for your library
  - Edit `internal/pkg/wiring/wiring.go` - Add your struct to the `ProviderSet` list
  - Edit `internal/pkg/wiring/wire.go` - Add an `Initialize` function for you struct

## Adding a Library (non-internal)

1. Add a new package for your library under `pkg`
1. Add a new package that contains the implementation under `internal/pkg`
  - Invoke it from your public package
