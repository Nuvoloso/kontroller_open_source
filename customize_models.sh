#!/bin/bash -e
# Copyright 2019 Tad Lebeck
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# This script works around an issue with swagger 0.20.1 that disallows "zero" values
# in map types. For example, sync_peers_map has additionalProperties whose value
# is of type sync_peer, which is a struct. If the sync_peer is empty, the model
# validator for sync_peers_map will reject it, even though this is valid.
#
# This script works around this by walking the generated models and
# removing this unnecessary validation.

for f in `echo pkg/autogen/models/*go`; do
    sed -e '/validate.Required.*k, "body"/,/}/d' < $f > sed_out
    chmod 644 sed_out
    if cmp -s sed_out $f; then
        true # the file was not modified
    else
        mv -f sed_out $f
        # remove import of validate package if it is no longer needed
        goreturns -w $f
    fi
done
rm -f sed_out # just in case
