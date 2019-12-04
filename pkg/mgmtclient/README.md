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

# mgmtclient

This package provides an interface over the auto-generated Nuvoloso client API and related functionality.

- The [API interface](api.go) is the low level management client interface, abstracting the auto-generated client API.
- The [Client](client.go) implements the API interface.
- The [CRUD Client](object/client.go) wraps the lower level Client API in a unified Create/Read/Update/Delete interface.
- The [Fake CRUD Client](fake/object_client.go) is a fake implementation of the CRUD Client for unit testing.
- The [UnixClient](unix_client.go) provides an [http.RoundTripper](https://golang.org/pkg/net/http/#RoundTripper) implemented
  over a Unix-domain socket for use with the management Client.
