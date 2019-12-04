# kontroller
Copyright 2019 Tad Lebeck

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Nuvoloso Control Software

This repository contains the source for the following programs:

- [catalog](./cmd/catalog) A tool to get snapshot information stored in the catalog
- [copy](./cmd/copy) A backup/restore tool that moves data around
- [mdDecode](./cmd/mdDecode) A metadata decoding utility
- [mdEncode](./cmd/mdEncode) A metadata encoding utility
- [mountfs](./cmd/mountfs) A development tool to test mounting and unmounting volumes
- [nuvo](./cmd/k8s/nuvo_fv) The Kubernetes FlexVolume driver
- [nuvo_vm](./cmd/nuvo_vm) The volume manager local command line (and associated library for nvagentd)
- [nvagentd](./cmd/agentd) The node controller daemon
- [nvauth](./cmd/auth) The central NV authentication service
- [nvaws](./cmd/aws) A development tool to exercise the portions of the AWS SDK of interest to us
- [nvazure](./cmd/azure) A development tool to exercise the portions of the Azure SDK of interest to us
- [nvcentrald](./cmd/centrald) The central controller daemon
- [nvclmd](./cmd/clmd) A development tool to discover details of a cluster
- [nvclusterd](./cmd/clusterd) The cluster controller daemon
- [nvctl](./cmd/ctl) The management CLI (**Perpetually under construction**)
- [nvgcp](./cmd/gcp) A development tool to exercise the portions of the Google SDK of interest to us
- [nvimd](./cmd/imd) A development tool to discover details of a Cloud Service Provider instance
- [nv-mongo-sidecar](./cmd/k8s/mongo-sidecar) A Kubernetes sidecar used to configure and manage a mongoDB replica set
- [snaplist](./cmd/snaplist) **DEPRECATED** A utility to list snapshots in the protection store

## Building

### Pre-requisites

The kontroller repo can be cloned anywhere. Common locations are `$GOPATH/src/github.com/Nuvoloso/kontroller` or `$HOME/src/kontroller`.
Older versions of [go](https://golang.org/) required the former path, but this restriction was removed with version `go1.13`.

The following tools are required to build this repository:

| Tool | Version | Debian Package | Linux(x86_64) notes |Mac Notes |
|------|:-------:|----------------|-----------|-----------|
| [make](https://www.gnu.org/software/make/) | 3.81 or above |  make | sudo apt-get install make | xcode-select --install |
| [go](https://golang.org/)   | 1.13.1 |   *none* | https://dl.google.com/go/go1.13.1.linux-amd64.tar.gz |  *none* |
| [(go)swagger](https://github.com/go-swagger/go-swagger) | 0.20.1 |  (see [link](https://github.com/go-swagger/go-swagger)) | tools/gobin/linux_amd64/swagger.v0.20.1 | see below |
| [protoc-gen-go](https://github.com/golang/protobuf) | 0.0~git20150526-2 | golang-goprotobuf-dev | sudo apt-get install golang-goprotobuf-dev | see [link](https://github.com/golang/protobuf) and see below |
| [protobuf](https://github.com/google/protobuf) |3.5.0 or above | *ignore* | Download pre-built release from https://github.com/google/protobuf/releases/download/v3.5.0/protoc-3.5.0-linux-x86_64.zip (or later version URL) and manually install in **/usr/local/bin** and **/usr/local/include**. Read [Install protobuf 3 on Ubuntu](https://gist.github.com/sofyanhadia/37787e5ed098c97919b8c593f0ec44d8) for background.| brew install protobuf |

### Mac OS or Linux Development Environment

Tools such as `swagger`, `mockgen` and `protoc-gen-go` are required to develop the kontroller repo. These tools and many more can be
accessed by cloning the [tools repo](https://github.com/Nuvoloso/tools) and adding the
`tools/gobin/darwin_amd64` or `tools/gobin/linux_amd64` directory to your `$PATH`, depending on your platform -- Mac OS or Linux respectively --
at or near the front of your `$PATH` to ensure you are using the correct version of the tools. Other common, recommended development tools
such as `dlv`, `golint` and `goreturns` are also provided.

All go code (other than auto-generated code) should be formatted using the [goreturns](https://github.com/sqs/goreturns) tool.
If you use the recommended [VS Code](https://code.visualstudio.com) for development, the editor can be configured to automatically format.
However, if you use a different tool you will need to format it manually.

#### Manually installing Mac OS Development Environment tools

These steps are only required on Mac OS if you do not add the `tools/gobin/darwin_amd64` directory to your $PATH (similar steps would be required on Linux, but you really should just update your `$PATH` as documented above).

To `make build`, the go swagger v0.20.1 binary is required.
```
$ curl -Lo $GOPATH/bin/swagger https://github.com/go-swagger/go-swagger/releases/download/v0.20.1/swagger_darwin_amd64
$ chmod +x $GOPATH/bin/swagger.v0.20.1
```
To regenerate mocks such as those in `pkg/centrald/mongods` you will also need [gomock](https://github.com/golang/mock).
The link above has installation instructions.  Our build requires a particular version of mock.
To install this version you should perform these steps:
```
go get -d github.com/golang/mock/mockgen
cd $GOPATH/src/github.com/golang/mock/mockgen && git checkout v1.3.1
go install github.com/golang/mock/mockgen
```

If you follow the instructions above you may get an improper version of go protobuf tools.
You know you have this problem if `make build` fails with an error like
```
pkg/nuvoapi/nuvo_pb/nuvo.pb.go:23:11: undefined: proto.ProtoPackageIsVersion3
```
Resolve by
```
cd $GOPATH/src/github.com/golang/protobuf/protoc-gen-go
git checkout tags/v1.2.0 -b v1.2.0
go install
```

### To build
Type
```
make
```
in the top directory to recreate the vendor dependency directory and then the programs in this repository.

During development one can build from within the individual `cmd/program` directories too.

To regenerate mocks such as those in `pkg/centrald/mongods` you can `make mock` within those specific directories.

### To test
Type
```
make test
```
in the top directory to run unit tests.  This step assumes that the vendor dependencies are up-to-date.

### A note on dependencies
We use [go modules](https://blog.golang.org/using-go-modules) to track dependencies. Versions of known dependencies
are tracked in the [go.mod](./go.mod) file. Their validity is tracked in the [go.sum](./go.sum) file. On occasion,
dependencies may need to be updated. Note that the dependent packages are stored locally below `$HOME/go/pkg/mod`.
Dependencies are automatically downloaded when they are referenced by the `go` tools.

The auto-generated `swagger` code has references
to some external packages that do not use any form of versioning!
In such cases specify the commit versions that have been found to work to the [go.mod](./go.mod) file.
The [go.sum](./go.sum) file is checked in for this same reason to ensure
reproducible builds over time.

## Documentation
One can view the Nuvoloso API specification in a browser as follows:
```
make docs
```
This runs a `swagger` server that attempts to launch your local browser to the appropriate runtime URL.
The URL is also displayed on the terminal so you can connect to it manually if the launch did not succeed.

The *classic swagger* documentation interface can be viewed with:
```
make docs_classic
```
It is less pretty but offers a `curl` command construction that may be useful during development.

The *go language* documentation can be viewed by launching the `godoc` server on port 6060 with
```
make godocs
```
and then pointing your browser to http://localhost:6060/pkg/github.com/Nuvoloso/kontroller/.
