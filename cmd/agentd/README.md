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

# nvagentd daemon

This is the main package of the per-node agent Nuvoloso management daemon.  The structure of this program is almost identical to [centrald](../centrald).

## Running nvagentd during development

After the build is complete, you can run nvagentd from the top level directory.

```
./cmd/agentd/nvagentd
```

See `./cmd/agentd/nvagentd --help` for its complete usage. It may be convenient to use an ini file to configure it.
If present, the ini file must be located at `/etc/nuvoloso/nvagentd.ini`.
You can generate a template ini file using the `--write-config=nvagentd.ini`, then edit and move it to `/etc/nuvoloso/nvagentd.ini`.
