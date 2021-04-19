# Bully election algorithm

[![CodeQL](https://github.com/iskorotkov/bully-election/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/iskorotkov/bully-election/actions/workflows/codeql-analysis.yml)

Bully election algorithm for distributed systems implemented in Go for deploying in Kubernetes clusters.

- [Bully election algorithm](#bully-election-algorithm)
  - [Monitor](#monitor)
  - [Build](#build)
  - [Deploy](#deploy)
  - [Limitations](#limitations)
  - [Links](#links)

## Monitor

1. `/metrics` - use metrics endpoint to get info about replica state,
1. Bully election dashboard - [web app for monitoring entire cluster](https://github.com/iskorotkov/bully-election-dashboard).

## Build

```sh
make build # to build locally.
make test # to run all tests.
make build-image # to build Docker image.
```

## Deploy

```sh
make deploy # to deploy in Kubernetes cluster.
```

## Limitations

1. sometimes cluster selects wrong leader on deployment creation,
1. when number of pods is high (>=20) there may be several leaders selected.

## Links

- [Bully algorithm - Wikipedia](https://en.wikipedia.org/wiki/Bully_algorithm)
- [Bully Election Algorithm Example](https://www.cs.colostate.edu/~cs551/CourseNotes/Synchronization/BullyExample.html)
- [Alternative implementation (Go)](https://github.com/TimTosi/bully-algorithm)
