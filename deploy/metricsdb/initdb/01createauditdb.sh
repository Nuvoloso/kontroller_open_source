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

PSQL="psql -U postgres"

$PSQL -tc "select 1 from pg_database where datname = 'nuvo_audit'" |
grep -q 1 || $PSQL -v ON_ERROR_STOP=1 -c "create database nuvo_audit"
