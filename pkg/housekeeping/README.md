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

# The housekeeping package
This package provides support for **in-memory tasks** that are partially managed through the REST API.
The tasks do not persist beyond the lifetime of the enclosing service process.

## Task
A housekeeping task is represented by a **models.Task** object defined in the Swagger specification.
The specification should be updated to add additional properties as needed.

Support for a task is declared by registering an *Animator* for the task.
It is used to:
- Validate the creation arguments
- Run the task

Task objects are created in one of two ways:
- In the same process by directly calling the package API
- In the same or a remote process by calling its REST API

Tasks are executed on individual go routines via the **TaskAnimator** interface.
They are provided with a **TaskOps** interface with methods to
- Get the models.Task object
- Set the state of the task
- Set task progress
- Check if the task has been canceled by some external entity

Task execution is non-preemptive; long lived tasks should periodically check if the task has been canceled.
The animator is also provided a context that is canceled when the manager is terminated.

## Manager
A Manager object must be created to coordinate house keeping activities.
It implements the **TaskScheduler** interface that provides methods to:
- Define an animator for each supported task operation
- Register the REST API handlers in the NuvolosoAPI (Create, List and Cancel support)
- Run internal tasks
- List internal tasks (optionally filtered by operation)
- Cancel internal tasks
- Terminate the manager and cancel the context provided to each task

While it is possible to create multiple Manager objects in the same process, it is not possible to
register multiple manager REST API handlers in the same NuvolosoAPI.
