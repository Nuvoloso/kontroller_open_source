#!/usr/bin/env bash
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

set -e

# This script replaces the default entrypoint script (docker-entrypoint.sh) and
# emulates some of its actions.
# If invoked on "postgres" it will call the original script if not previously
# initialized, otherwise it will invoke the schema scripts to perform an upgrade.

if [ "${1:0:1}" = '-' ]; then
	set -- postgres "$@"
fi

if [ "$1" = 'postgres' ]; then
    if [ ! -s "$PGDATA/PG_VERSION" ]; then
        # this covers the case of an uninitialized container
        exec docker-entrypoint.sh "$@"
    fi
    echo "Updating schema if necessary (" $(id -n -u) ")"

    # the following emulates docker-entrypoint.sh
    if [ "$(id -u)" = '0' ]; then
	    chown -R postgres "$PGDATA"
	    chmod 700 "$PGDATA"
        exec su-exec postgres "$BASH_SOURCE" "$@"
    fi
    PGUSER="${PGUSER:-postgres}" \
            pg_ctl -D "$PGDATA" \
			-o "-c listen_addresses='localhost'" \
			-w start
    psql=( psql -v ON_ERROR_STOP=1 )
    for f in /docker-entrypoint-initdb.d/*; do
        case "$f" in
            *.sh)     echo "$0: running $f"; . "$f" ;;
            *.sql)    echo "$0: running $f"; "${psql[@]}" -f "$f"; echo ;;
            *)        echo "$0: ignoring $f" ;;
        esac
        echo
    done
    PGUSER="${PGUSER:-postgres}" \
            pg_ctl -D "$PGDATA" -m fast -w stop
fi

exec "$@"
