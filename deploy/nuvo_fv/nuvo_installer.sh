#!/bin/sh
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

# Install the nuvo flexvolume driver in the kubernetes volume plugin directory

set -e

[ -z "$VOLUME_PLUGIN_PATH" ] && echo VOLUME_PLUGIN_PATH is not set && exit 1
if [ -z "$MANAGEMENT_SOCK" ]; then
    export MANAGEMENT_SOCK=/var/run/nuvoloso/nvagentd.sock
fi

[ -d "$VOLUME_PLUGIN_PATH" ] || { echo VOLUME_PLUGIN_PATH $VOLUME_PLUGIN_PATH is not a directory; exit 1; }

echo nuvo installer run with
echo '   ' VOLUME_PLUGIN_PATH=$VOLUME_PLUGIN_PATH
echo '   ' MANAGEMENT_SOCK=$MANAGEMENT_SOCK
echo install time: `date +%FT%T%z`

SOCKOPT="SocketPath = $MANAGEMENT_SOCK"
cat > nuvo.ini << EOF
[Application Options]
$SOCKOPT
EOF

NUVODIR="$VOLUME_PLUGIN_PATH/nuvoloso.com~nuvo"
[ -d "$NUVODIR" ] || { mkdir "$NUVODIR" && echo created "$NUVODIR"; }

grep -sq "^${SOCKOPT}\$" "$NUVODIR/nuvo.ini" || { cp nuvo.ini "$NUVODIR/nuvo.ini" && chmod 644 "$NUVODIR/nuvo.ini" && echo installed or updated "$NUVODIR/nuvo.ini"; }
cmp -s /opt/nuvoloso/bin/nuvo "$NUVODIR/nuvo" || { cp /opt/nuvoloso/bin/nuvo "$NUVODIR/.nuvo" && chmod 755 "$NUVODIR/.nuvo" && mv -f "$NUVODIR/.nuvo" "$NUVODIR/nuvo" && echo installed or updated "$NUVODIR/nuvo"; }

"$NUVODIR/nuvo" version; echo ''
echo Successfully installed the nuvo driver
exit 0
