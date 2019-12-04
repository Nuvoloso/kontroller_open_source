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

# Cloud Service Provider abstraction

This package provides interfaces to abstract common services offered by cloud service providers.
Only time will tell if we got it right...

Each specific cloud service provider support is provided in a sub-package.
These sub-packages must register their implementation of the abstract interface from an init() function.
The sub-packages must be imported by the program main to become available for use.
