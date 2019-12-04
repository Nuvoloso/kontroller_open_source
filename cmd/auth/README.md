# nvauth service

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

This is the main package of the central Nuvoloso authentication service.

## Running nvauth during development

After the build is complete, you can run nvauth from the top level directory

```
./cmd/centrald/nvauth
```

nvauth process will be running on port 5555, under /auth. If running locally, make REST API requests to http://localhost:5555/auth/.

## Authentication API specification
Currently supported authentication APIs are:
```
POST /login
Body
{"username": "admin", "password": "admin"}
Successful response body (status 200)
{"token": "string", "exp": {utc timestamp in ms}, "iss": "Nuvoloso", "username": "admin"}
Failure response text/plain body (status 401)
Error message as a string
```
Note: The expiration time returned is 2 hours in the future. This is not currently configurable.

```
POST /logout
```
Note: This request will mark the current client session as unauthenticated. Subsequent requests to validate using the same client will return an error, even if that token's expiration has not passed. That specific client will need to re-authenticate for a new token using `POST /auth/login`.

```
POST /validate
Header
key: Token
value: {token string from response of login}
Successful response body (status 200)
{"token": "string", "exp": {utc timestamp in ms}, "iss": "Nuvoloso", "username": "admin"}
Failure response text/plain body (status 401)
Error message as a string
```
Note: The successful response for this request will return a new token with a new expiration time of 2 hours later from time of validation.
To override this behavior, pass an optional `preserve-expiry=true` query parameter, eg `POST /validate?preserve-expiry=true`.
