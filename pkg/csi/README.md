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

# CSI support

This package offers mechanisms to interact with Kubernetes CSI sidecars
(see [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/) for details).

## ControllerHandler
This is an interface used for dynamic provisioning, snapshots and volume deletion.
It should be used in **clusterd** and requires the following sidecars to be accessible:
- [cluster-driver-registrar](https://kubernetes-csi.github.io/docs/cluster-driver-registrar.html)
- [external-provisioner](https://kubernetes-csi.github.io/docs/external-provisioner.html) *Future: required for dynamic provisioning*
- [external-snapshotter](https://kubernetes-csi.github.io/docs/external-snapshotter.html) *Future: required for snapshots*
- [livenessprobe](https://kubernetes-csi.github.io/docs/livenessprobe.html)

The [external-attacher](https://kubernetes-csi.github.io/docs/external-attacher.html) is not used.
The interface is returned by the **NewControllerHandler()** function.

## NodeHandler
This is an interface used to for volume mount and unmount.
It should be used in **agentd** and requires the following sidecars to be accessible:
- [node-driver-registrar](https://kubernetes-csi.github.io/docs/node-driver-registrar.html)
- [livenessprobe](https://kubernetes-csi.github.io/docs/livenessprobe.html)

The interface is returned by the **NewNodeHandler()** function.
