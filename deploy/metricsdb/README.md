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

# The metrics database container

The metrics database container contains a database engine and configuration files packaged into a single Docker container.
The entrypoint of this container is the nuvo-entrypoint.sh script, which initializes the container on first use,
installing or updating the schema.
An external filesystem should be mounted at **/var/lib/postgressql/data** to store the data files persistently.

The [TimeScale](https://www.timescale.com/) database engine is used as it offers *hypertables*
in which to efficiently store, access and manage time-series data.
It is an extension of [Postgres](https://www.postgresql.org/), offering full SQL capabilities along with
additional commands specific to hypertables.

The metrics database container is currently configured with the following databases:

- Metrics data (in the *nuvo_metrics* database)
- Audit logs (in the *nuvo_audit* database)

## Schema testing
During development the schema can be tested locally by running the container.
A local directory must be specified for data storage.

To build the container locally first ensure that [docker](https://docs.docker.com/engine/reference/commandline/docker/)
is available on the local machine.
Then build the container from the root directory of the kontroller repository as follows:
```
$ docker build -f deploy/Dockerfile.metricsdb -t metricsdb:latest .
```

This creates the container and tags it with *"metricsdb:latest"*.
The following will run this container:
```
$ DBNAME=mdb
$ DBDIR=~/db
$ mkdir -p $DBDIR
$ docker run -p 5432:5432 --name $DBNAME -v $DBDIR:/var/lib/postgressql/data metricsdb:latest
```
The database engine is started with the standard [Postgres](https://www.postgresql.org/) port number
and store the data files in **$DBDIR**.

Use `psql` in another terminal to connect to the database engine. Either install `psql` locally or use the copy in the container.
For example:
```
$ psql -h localhost -U postgres -d nuvo_metrics
```
can be used to connect to the metrics database.

The container can be stopped with
```
$ docker stop $DBNAME
```
and restarted with
```
$ docker start $DBNAME
```
