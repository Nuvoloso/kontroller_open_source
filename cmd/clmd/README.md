# license

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

# nvclmd

This is a tool to display Cluster meta-data.
**It is not intended for production!**

For example, in a Kubernetes instance:
```
root@shell-demo:/# /nvclmd
ClusterIdentifier: k8s-svc-uid:c981db76-bda0-11e7-b2ce-02a3152c8208
ClusterVersion: 1.7
CreationTimestamp: 2017-10-30T18:33:13Z
GitVersion: v1.7.8
Platform: linux/amd64
```

The command will timeout if launched in a non-cluster environment.
