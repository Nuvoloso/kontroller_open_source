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

# nuvo flexvolume driver

This is the main package of the Nuvoloso Flexvolume driver for Kubernetes.

The driver is a CLI that the Kubernetes infrastructure uses to mount vendor-specific volumes.
See [this documentation](https://github.com/kubernetes/community/blob/master/contributors/devel/flexvolume.md) for more information.

## Use of the driver in development

TBD. The current driver implements the required API and fails all functions. It can be run in place after building.

## Use of the driver in production

In production, the driver must be installed as a root-accessible, executable in a path of
the form `<plugindir>/<vendor~driver>/<driver>`.
By default, the `<plugindir>` is `/usr/libexec/kubernetes/kubelet-plugins/volume/exec`.
Our vendor name is `nuvoloso.com` and the driver is called `nuvo`, so the full default path
is `/usr/libexec/kubernetes/kubelet-plugins/volume/exec/nuvoloso.com~nuvo/nuvo`.
