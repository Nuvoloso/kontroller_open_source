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

# nvcentral daemon

This is the main package of the central Nuvoloso management daemon.

## Running nvcentrald during development

By default, nvcentrald requires
that the `/opt/nuvoloso/lib/base-data/` directory exists containing the contents of the `deploy/centrald/lib` directory
found in the source repo. One way to ensure this is correct and current during development is to create a symlink such as:

```
$ sudo ln -s $GOPATH/src/github.com/Nuvoloso/kontroller/deploy/centrald/lib /opt/nuvoloso/lib
```

It is also possible to address this requirement by use of the `--mongo.base-data-path` flag, setting the value to the
full path to the `deploy/centrald/lib` source directory. Setting the value to the empty string is also possible,
at the cost of lost functionality (e.g. SLOs will not be populated).
This flag can also be set in the ini file, discussed below.

After the build is complete, you can run nvcentrald from the top level directory.

```
./cmd/centrald/nvcentrald
```

See `./cmd/centrald/nvcentrald --help` for its complete usage. It may be convenient to use an ini file to configure it.
If present, the ini file must be located at `/etc/nuvoloso/nvcentrald.ini`.
You can generate a template ini file using the `--write-config=nvcentrald.ini`, then edit and move it to `/etc/nuvoloso/nvcentrald.ini`.
