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

# Runtime Error Injection Support

This package provides support to inject errors dynamically at runtime under external control.
Essentially it provides support for "ephemeral property manager" objects that return values for
properties of some type; the default value for that type should be used to code normal behavior.

The ephemeral nature comes from a property value being automatically cleared on access.
For example, the following could be used in a request to force failure once when desired.
The subsequent call made on retry of the request would once again return the default value of **false**:
```
if epm.GetBool("fail-on-spa-creation") {
    op.rhs.SetRequestMessage("forcing failure")
    op.rhs.RetryLater = true
    return
}
```

A property manager's response is controlled by the presence of specially named "property" files in a
configured directory (its "arena").
It is recommended that individual property managers each be configured with a private arena to separate
their property name spaces.
Support is provided to establish a global arena for the process as a whole with individual
property manager arenas relative to the global arena.

A property value is set via a file in the appropriate arena, created by a developer at run time to trigger
the use of some specific code path.
It contains a JSON object with the following fields:
- *boolValue, stringValue, intValue, floatValue* : Typed value fields
- *numUses* : The number of references that may be made to the property before it is purged from memory (at least 1 reference is guaranteed regardless of the value)
- *doNotDelete* : Boolean indicating that the property file should not be deleted

The file is looked for only when the property is referenced in code, as illustrated in the example above.
The returned value of a property is based on the method used to query it. In the case of a boolean the
presence of the property file itself will imply a **true** value if the file is empty; a
**false** value must be set via the file content.

For example, given the following property file
```
# cat /etc/nuvoloso/centrald/rei/vsr:ALLOCATE_CAPACITY/fail-on-account
{
    "stringValue": "carlb",
    "floatValue": 1.4,
}
```
the ```GetString("fail-on-account")``` method would return **"carlb"**,
the ```GetBool("fail-on-account")``` method would return **false** (because *boolValue* is not set),
the ```GetFloat("fail-on-account")``` method would return **1.4**,
and the ```GetInt("fail-on-account")``` method would return **0** (because *intValue* is not set).


Use of the ephemeral property managers must be explicitly enabled for the process as a whole and
on a per-manager basis in order for a manager to return anything other than the default value of a property.
If the package is enabled before configuring the global arena and creating the ephemeral property managers
then the arena directories are created by the package if not already present.
It is expected that command line flags will be used to control this.

To aid in unit testing, ephemeral property managers provide a *SetProperty* method that may explicitly set the
values to return regardless of the "enabled" settings.
