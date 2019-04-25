![grape](https://raw.githubusercontent.com/txn2/grape/master/mast.jpg)
[![op Release](https://img.shields.io/github/release/txn2/grape.svg)](https://github.com/txn2/grape/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/txn2/grape)](https://goreportcard.com/report/github.com/txn2/grape)
[![GoDoc](https://godoc.org/github.com/txn2/grape?status.svg)](https://godoc.org/github.com/txn2/grape)
[![Docker Container Image Size](https://shields.beevelop.com/docker/image/image-size/txn2/grape/latest.svg)](https://hub.docker.com/r/txn2/grape/)
[![Docker Container Layers](https://shields.beevelop.com/docker/image/layers/txn2/grape/latest.svg)](https://hub.docker.com/r/txn2/grape/)


WIP: TXN2 GRAfana Proxy for Elasticsearch data source.


## Release Packaging

Build test release:
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

Build and release:
```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```