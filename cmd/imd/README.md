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

# nvimd

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

This is a tool to display Cloud Service Provider instance data.
**It is not intended for production!**

For example, in an AWS instance:
```
ubuntu@ip-172-31-24-56:~$ /tmp/nvimd -D AWS
InstanceName: i-00cf71dee433b52db
LocalIP: 172.31.24.56
Zone: us-west-2a
instance-type: t2.micro
local-hostname: ip-172-31-24-56.us-west-2.compute.internal
public-hostname: ec2-34-212-187-125.us-west-2.compute.amazonaws.com
public-ipv4: 34.212.187.125
```

The command will timeout if launched in a non-CSP environment.
