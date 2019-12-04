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


package main

import (
	"context"

	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const systemCollection = "system"

// System object, only the ID is used
type System struct {
	ID primitive.ObjectID `bson:"_id"`
}

var systemObj = System{}

// getSystemID returns the ID of the system object on success, an error if it cannot be looked up.
// The caller is expected to perform synchronization
func getSystemID(ctx context.Context, api mongodb.DBAPI) ([]byte, error) {
	if systemObj.ID.IsZero() {
		if api.MustBeReady() != nil {
			return nil, errNotReady
		}
		if err := api.Client().Database(api.DBName()).Collection(systemCollection).FindOne(ctx, bson.D{}).Decode(&systemObj); err != nil {
			api.ErrorCode(err)
			api.Logger().Error("FindOne", err)
			return nil, errNotReady
		}
		api.ErrorCode(nil)
	}
	return systemObj.ID[:], nil
}
