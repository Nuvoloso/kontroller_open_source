# License

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

# Kubernetes MongoDB sidecar

This is the main package of the Nuvoloso MongoDB sidecar. The sidecar is deployed along side the MongoDB containers in
a Kubernetes [StatefulSet](https://Kubernetes.io/docs/concepts/workloads/controllers/statefulset/). Its purpose is to configure,
monitor and manage the MongoDB replica set. It supports 1 and 3 replicas.
At present, it does not support scaling, nor does it actually monitor or manage the configured replica set.

It can detect when the replica set is newly created and initializes it. In the future it may be enhanced to detect certain
unhealthy situations and automatically recover from them. It may also be used as an entry point to back up the Nuvoloso
configuration database stored in the replica set.

## Usage

In development one can build and execute `cmd/k8s/mongo-sidecar/nv-mongo-sidecar`. However, only executing while specifying either of
the `--help` or `--write-config` options is expected to work outside of a container in Kubernetes.

`nv-mongo-sidecar` can only perform its normal functions in a container in a Kubernetes StatefulSet.
It expects to be deployed in a pod of a StatefulSet that also contains a mongodb container.
The tools in the [deployment repo](https://github.com/Nuvoloso/deployment) can be used to deploy such a StatefulSet.
The `nv-mongo-sidecar` is configured by its command line parameters, environment variables and optionally a configuration file.

The following environment variables are recognized.

| Environment Variable | Required | Default | Description |
|----------------------|----------|---------|-------------|
| CLUSTER_DNS_NAME | N | cluster.local | Top part of the Kubernetes stable DNS name used in this cluster |
| MONGODB_RS_NAME | Y | | Name of the MongoBD replica set. Must be the same as the value passed on the mongod container command line |
| NUVO_NODE_NAME | N | | (Future) The node where the pod is executing (i.e. `spec.nodeName`) for generating Kubernetes events |
| NUVO_POD_IP | Y | | The pod's IP address (i.e. `status.podIP`) |
| NUVO_POD_NAME | Y | | The pod name (i.e. `metadata.name`) |
| NUVO_POD_NAMESPACE | Y | | The namespace containing the pod (i.e. `metadata.namespace`) |
| NUVO_POD_UID | N | | (future) the Pod unique identifier (i.e. `metadata.uid`) for generating Kubernetes events |
