module github.com/Nuvoloso/kontroller

// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


go 1.13

require (
	cloud.google.com/go/storage v1.0.0
	contrib.go.opencensus.io/exporter/stackdriver v0.0.0-20180815171500-f7f575859225 // indirect
	github.com/Azure/azure-sdk-for-go v34.1.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.8.0
	github.com/Azure/go-autorest v13.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.9.2
	github.com/Azure/go-autorest/autorest/adal v0.7.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.0
	github.com/Azure/go-autorest/autorest/to v0.3.0
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/DATA-DOG/go-sqlmock v1.3.3
	github.com/Masterminds/semver v1.5.0
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf
	github.com/aws/aws-sdk-go v1.23.22
	github.com/aws/aws-sdk-go-v2 v0.0.0-20180816233432-295bebb2e006
	github.com/cockroachdb/apd v1.1.0 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/docker/go-units v0.4.0
	github.com/fogleman/gg v1.1.0
	github.com/go-ini/ini v1.25.4
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/jsonreference v0.19.3 // indirect
	github.com/go-openapi/loads v0.19.3
	github.com/go-openapi/runtime v0.19.6
	github.com/go-openapi/spec v0.19.3
	github.com/go-openapi/strfmt v0.19.3
	github.com/go-openapi/swag v0.19.5
	github.com/go-openapi/validate v0.19.3
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/golang/snappy v0.0.1 // indirect
	github.com/gorilla/context v0.0.0-20160226214623-1ea25387ff6f
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/gorilla/sessions v0.0.0-20160922145804-ca9ada445741
	github.com/gorilla/websocket v1.4.0
	github.com/jackc/fake v0.0.0-20150926172116-812a484cc733 // indirect
	github.com/jackc/pgx v3.5.0+incompatible
	github.com/jessevdk/go-flags v1.4.0
	github.com/julienschmidt/httprouter v1.1.0
	github.com/lib/pq v1.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.0-20170510074858-97311d9f7767 // indirect
	github.com/olekukonko/tablewriter v0.0.0-20170925234030-a7a4c189eb47
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pkg/errors v0.0.0-20171216070316-e881fd58d78e // indirect
	github.com/satori/go.uuid v0.0.0-20170321230731-5bf94b69c6b6
	github.com/shopspring/decimal v0.0.0-20190905144223-a36b5d85f337 // indirect
	github.com/smartystreets/goconvey v0.0.0-20190731233626-505e41936337 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c // indirect
	github.com/xdg/stringprep v1.0.0 // indirect
	go.mongodb.org/mongo-driver v1.1.1
	golang.org/x/crypto v0.0.0-20190820162420-60c769a6c586
	golang.org/x/net v0.0.0-20191009170851-d66e71096ffb
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	google.golang.org/api v0.11.0
	google.golang.org/appengine v1.6.4 // indirect
	google.golang.org/genproto v0.0.0-20190927181202-20e1ac93f88c // indirect
	google.golang.org/grpc v1.24.0
	gopkg.in/yaml.v2 v2.2.2
)
