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

# nvctl

This is the command line interface to manage the Nuvoloso system.

## Short to long flag mapping

| Short | Long | Location |
|-------|------|------|
| -A | account | main (applies to all) |
| -a | action | audit |
|    | all | auth |
|    | attribute | cspdomain |
|    | scope | pool, storage |
| -B | timestamp-le | audit |
| -b | min-size-bytes | storage request |
|    | size-bytes | volume series request |
|    | total-bytes | pool |
| -C | cluster-name | account, audit, node, pool, spa, storage storage request, volume series, volume series request |
| -c | columns | *all* |
| -D | domain | cluster, node, pool, spa, snapshot, storage, storage request, volume series, volume series request |
|    | domain-name | account, cluster |
| -d | description | account, cluster, cspdomain, protection domain, system, volume series, volume series request |
|    | data | cluster |
| -E | encryption-algorithm | protection domain |
| -F | args-file | watcher |
|    | cred-file | cspcredential |
|    | format | cluster |
|    | fs-type | volume series request |
|    | storage-formula-name | spa |
| -f | follow-changes | cluster, cspdomain, node, pool, spa, snapshot, storage, storage request, task, volume series, volume series request |
|    | forget-password | auth |
| -H | management-host | cspdomain |
| -h | help | *all* |
| -I | auth-identifier | user |
|    | cluster-identifier | cluster |
|    | node-identifier | node, pool |
| -K | descriptor-key | spa, volume series |
| -k | ssl-skip-verify | main (applies to all) |
| -L | last-event-file | watcher |
| -M | method-pattern | watcher |
| -m | message | audit |
| -N | mounted-node-name | volume series |
|    | new-auth-identifier | user |
|    | new-name | account, cluster, cspdomain, system |
|    | node-name | storage, storage request,volume series,volume series request |
| -n | name | account, application group, audit, cluster, consistency group, cspdomain, node, protection domain, role, service plan, slo, storage formula, volume series, volume series request |
| -O | operation | volume series request |
|    | operations | storage request |
|    | output-file | cluster, cspdomain spa, volume series |
| -o | output | *all* |
| -P | pit-id | snapshot |
|    | pool-id | storage, storage request |
|    | profile | user |
|    | service-plan | cspdomain, spa, volume series, volume series request |
| -p | parcel-size-bytes | storage request |
|    | parent | audit |
|    | password | auth, user |
|    | pass-phrase | protection domain |
| -q | silent | watcher |
| -R |active-or-modified-ge | storage request, volume series request |
|    |recent-snapshots | snapshot |
|    |timestamp-ge | audit |
| -r | recursive | account |
|    | related | audit |
| -S | account-secret-scope | account, cluster, cspdomain, system |
|    | scope-pattern | watcher |
|    | snap-id | snapshot |
|    | snapshot-id | volume series request |
|    | stack | debug |
|    | storage-id | storage request |
|    | subordinate-account | audit |
| -s | store-password | auth |
|    | source-service-plan | spa |
|    | state | volume series request |
| -T | domain-type | cspdomain, storage type |
|    | cluster-type | cluster |
|    | storage-type | pool, storage |
|    | type | audit |
| -t | tag | account, application group, cluster, consistency group, cspdomain, node, pool, protection domain, service plan, spa, snapshot, volume series, volume series request |
| -U | uri-pattern | watcher |
| -u | user | audit |
|    | user-roles | account |
| -V | version | account, cluster, consistency group, cspdomain, service plan, spa, system, user, volume series |
|    | volume-series | snapshot |
|    | volume-series-id | volume series request |
| -v | verbose | main (applies to all) |
| -x | complete-by | storage request, volume series request |
|    | skip-validation | auth |
| -Z | auth-account-name | service plan, spa |
|    | authorized-account | cluster, cspdomain, pool, service plan, spa, volume series request |

